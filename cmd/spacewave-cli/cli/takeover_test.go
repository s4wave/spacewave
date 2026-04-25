//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// TestTakeoverDaemonSocketShutsDownDesktopListener asserts that
// takeoverDaemonSocket cleanly shuts down a listener configured the
// same way the desktop resource listener (core/resource/listener) is:
// a single mux registering the daemon-control handler. The listener
// side removes its socket file on shutdown, and the socket must be
// reusable by a subsequent listen.
func TestTakeoverDaemonSocketShutsDownDesktopListener(t *testing.T) {
	ctx := t.Context()

	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-a"), "desktop.sock")
	lis := startDesktopLikeListener(t, ctx, sock)

	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	select {
	case <-lis.done:
	case <-time.After(5 * time.Second):
		t.Fatal("desktop listener did not exit after takeover")
	}

	// The desktop listener removes its socket on exit; verify a fresh
	// listen on the same path succeeds (no orphan file).
	newLis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("relisten after takeover: %v", err)
	}
	newLis.Close()
	_ = os.Remove(sock)
}

// TestTakeoverDaemonSocketRemovesStaleSocket asserts takeover removes
// a leftover socket file when nothing is listening on it.
func TestTakeoverDaemonSocketRemovesStaleSocket(t *testing.T) {
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-b"), "stale.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("expected socket removed; stat err=%v", err)
	}
}

// TestTakeoverDaemonSocketNoop asserts takeover succeeds silently when
// no socket file exists.
func TestTakeoverDaemonSocketNoop(t *testing.T) {
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-c"), "missing.sock")

	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}
}

// makeShortTakeoverDir returns a short, test-package-local directory
// for Unix sockets. Darwin enforces a ~104 byte limit on sun_path;
// t.TempDir on macOS can exceed this once the test name is long.
func makeShortTakeoverDir(t *testing.T, name string) string {
	t.Helper()
	tmpRoot, err := filepath.Abs(".tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmpRoot, name)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

// desktopLikeListener simulates the core/resource/listener controller
// Execute flow for the purposes of daemon-control integration tests.
type desktopLikeListener struct {
	done chan struct{}
}

// startDesktopLikeListener spawns a unix socket listener with the
// daemon-control handler registered. On shutdown the listener removes
// its socket file (matching the controller's defer cleanup).
func startDesktopLikeListener(t *testing.T, ctx context.Context, sock string) *desktopLikeListener {
	t.Helper()

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	serveCtx, serveCancel := context.WithCancel(ctx)
	mux := srpc.NewMux()
	if err := mux.Register(newDaemonControlHandler(func() {
		serveCancel()
		lis.Close()
	})); err != nil {
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
