//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	"github.com/sirupsen/logrus"
)

// TestTakeoverDaemonSocketAllowsWhenPolicyAllows asserts that a
// broker-backed policy resolving Allow lets takeover complete
// cleanly, matching the desktop app's "user clicked Allow" flow.
func TestTakeoverDaemonSocketAllowsWhenPolicyAllows(t *testing.T) {
	ctx := t.Context()

	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-allow"), "d.sock")
	broker := yield_policy.NewBrokerWithTimeout(5 * time.Second)
	lis := startBrokerBackedListener(t, ctx, sock, broker)

	// Simulate the user clicking Allow when the prompt arrives.
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			prompts, waitCh := broker.SnapshotPrompts()
			if len(prompts) > 0 {
				_ = broker.ResolvePrompt(prompts[0].ID, true)
				return
			}
			select {
			case <-waitCh:
			case <-time.After(time.Until(deadline)):
				return
			}
		}
	}()

	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	select {
	case <-lis.done:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not exit after allow")
	}
}

// TestTakeoverDaemonSocketSurfacesDenyAsClearError asserts that a
// denied policy propagates back to takeoverDaemonSocket as a CLI
// error that names the Spacewave desktop app, matching the scope
// requirement.
func TestTakeoverDaemonSocketSurfacesDenyAsClearError(t *testing.T) {
	ctx := t.Context()

	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-deny"), "d.sock")
	broker := yield_policy.NewBrokerWithTimeout(5 * time.Second)
	startBrokerBackedListener(t, ctx, sock, broker)

	// Simulate the user clicking Deny.
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			prompts, waitCh := broker.SnapshotPrompts()
			if len(prompts) > 0 {
				_ = broker.ResolvePrompt(prompts[0].ID, false)
				return
			}
			select {
			case <-waitCh:
			case <-time.After(time.Until(deadline)):
				return
			}
		}
	}()

	le := logrus.NewEntry(logrus.New())
	err := takeoverDaemonSocket(ctx, le, sock)
	if err == nil {
		t.Fatal("expected takeover to fail on deny")
	}
	if !strings.Contains(err.Error(), "Spacewave desktop app") {
		t.Fatalf("deny error missing app name: %v", err)
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Fatalf("deny error missing denied: %v", err)
	}
}

// startBrokerBackedListener spawns a unix socket listener configured
// with a broker-driven yield policy, mirroring the desktop resource
// listener. The returned handle's done channel closes after shutdown.
func startBrokerBackedListener(
	t *testing.T,
	ctx context.Context,
	sock string,
	broker *yield_policy.Broker,
) *desktopLikeListener {
	t.Helper()

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	serveCtx, serveCancel := context.WithCancel(ctx)
	mux := srpc.NewMux()
	policy := broker.MakePolicy("spacewave serve", sock)
	shutdown := func() {
		serveCancel()
		lis.Close()
	}
	if err := mux.Register(listener_control.NewHandler(policy, shutdown)); err != nil {
		lis.Close()
		t.Fatalf("register control: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			lis.Close()
			_ = os.Remove(sock)
		}()
		server := srpc.NewServer(mux)
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				mp, err := srpc.NewMuxedConn(conn, false, nil)
				if err != nil {
					conn.Close()
					return
				}
				_ = server.AcceptMuxedConn(serveCtx, mp)
			}(conn)
		}
	}()

	t.Cleanup(func() {
		serveCancel()
		lis.Close()
	})
	return &desktopLikeListener{done: done}
}
