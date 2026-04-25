//go:build !js

package control

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// TestHandlerMetadata asserts the handler advertises the expected
// service and method identifiers.
func TestHandlerMetadata(t *testing.T) {
	h := NewHandler(nil, func() {})
	if got := h.GetServiceID(); got != ServiceID {
		t.Fatalf("service id = %q, want %q", got, ServiceID)
	}
	methods := h.GetMethodIDs()
	if len(methods) != 1 || methods[0] != ShutdownMethodID {
		t.Fatalf("method ids = %v, want [%s]", methods, ShutdownMethodID)
	}
}

// TestInvokeMethodIgnoresForeignRoutes asserts the handler returns
// unmatched (false, nil) for services and methods it does not own, so
// the mux keeps probing other handlers.
func TestInvokeMethodIgnoresForeignRoutes(t *testing.T) {
	h := NewHandler(nil, func() { t.Fatal("callback fired for foreign route") })
	handled, err := h.InvokeMethod("other.service", ShutdownMethodID, nil)
	if handled || err != nil {
		t.Fatalf("foreign service: handled=%v err=%v", handled, err)
	}
	handled, err = h.InvokeMethod(ServiceID, "Other", nil)
	if handled || err != nil {
		t.Fatalf("foreign method: handled=%v err=%v", handled, err)
	}
}

// TestShutdownRoundTrip asserts the Shutdown RPC round-trips between a
// starpc client and a handler-backed mux, and that the handler's
// shutdown callback fires exactly once.
func TestShutdownRoundTrip(t *testing.T) {
	ctx := t.Context()

	dir := t.TempDir()
	sock := filepath.Join(dir, "daemon.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		lis.Close()
		_ = os.Remove(sock)
	})

	shutdownCh := make(chan struct{}, 4)
	mux := srpc.NewMux()
	if err := mux.Register(NewHandler(nil, func() {
		shutdownCh <- struct{}{}
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := srpc.NewServer(mux)
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			conn.Close()
			return
		}
		_ = server.AcceptMuxedConn(ctx, mp)
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()
	if err := RequestShutdown(callCtx, conn); err != nil {
		t.Fatalf("request shutdown: %v", err)
	}

	select {
	case <-shutdownCh:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown callback did not fire")
	}
}

// TestShutdownDenyPropagates asserts a deny policy propagates to the
// caller as a DenyError without firing the shutdown callback.
func TestShutdownDenyPropagates(t *testing.T) {
	ctx := t.Context()

	dir := t.TempDir()
	sock := filepath.Join(dir, "daemon.sock")
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		lis.Close()
		_ = os.Remove(sock)
	})

	denyErr := errors.New("denied by Spacewave desktop app")
	shutdownFired := false
	mux := srpc.NewMux()
	policy := func(context.Context) error { return denyErr }
	if err := mux.Register(NewHandler(policy, func() {
		shutdownFired = true
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := srpc.NewServer(mux)
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			conn.Close()
			return
		}
		_ = server.AcceptMuxedConn(ctx, mp)
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()
	err = RequestShutdown(callCtx, conn)
	if err == nil {
		t.Fatalf("request shutdown: expected deny error, got nil")
	}
	var denyError *DenyError
	if !errors.As(err, &denyError) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if !strings.Contains(denyError.Reason, "Spacewave desktop app") {
		t.Fatalf("deny reason missing app name: %q", denyError.Reason)
	}
	if shutdownFired {
		t.Fatal("shutdown callback fired despite deny")
	}
}
