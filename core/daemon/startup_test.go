//go:build !js

package daemon

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
)

// TestStartupAcknowledgementFencesReadiness proves a public listener cannot
// become usable while the launcher still has authority to terminate its child.
func TestStartupAcknowledgementFencesReadiness(t *testing.T) {
	// Drive the notifier over its private pipe and observe the ready proposal.
	parent, child := net.Pipe()
	defer parent.Close()
	notifier := &StartupNotifier{conn: child}
	defer notifier.Close()
	ready := make(chan error, 1)
	go func() { ready <- notifier.Ready(t.Context()) }()
	message, err := bufio.NewReader(parent).ReadString('\n')
	if err != nil || message != "ready\n" {
		t.Fatalf("readiness proposal: %q, %v", message, err)
	}
	select {
	case err := <-ready:
		t.Fatalf("child served before custody transfer: %v", err)
	default:
	}

	// A canceled launcher closes the pipe without granting public readiness.
	_ = parent.Close()
	if err := <-ready; err == nil {
		t.Fatal("child accepted readiness without an acknowledgement")
	}
}

// TestWaitStartupTransfersCustody establishes the successful two-way handshake.
func TestWaitStartupTransfersCustody(t *testing.T) {
	// Use the real private pipe implementation on this platform.
	root := daemonTestRoot(t)
	listener, err := pipesock.BuildPipeListener(NewStartupPipeLogger(), root, "startup")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ready := make(chan error, 1)
	go func() {
		notifier, err := NewStartupNotifier(ctx, root, "startup")
		if err == nil {
			err = notifier.Ready(ctx)
		}
		ready <- err
	}()
	if err := WaitStartup(ctx, listener); err != nil {
		t.Fatal(err)
	}
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
}

// TestWaitStartupCancellationJoinsRead exercises cancellation after accept,
// when closing only the listening socket would strand the startup reader.
func TestWaitStartupCancellationJoinsRead(t *testing.T) {
	root := daemonTestRoot(t)
	listener, err := pipesock.BuildPipeListener(NewStartupPipeLogger(), root, "startup")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- WaitStartup(ctx, listener) }()
	conn, err := pipesock.DialPipeListener(t.Context(), NewStartupPipeLogger(), root, "startup")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: %v", err)
	}
}
