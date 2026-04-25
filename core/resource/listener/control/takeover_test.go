//go:build !js

package control

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

// TestTakeoverSocketShutsDownLiveDaemon asserts that TakeoverSocket
// issues the Shutdown RPC when a live daemon answers on the socket,
// waits for the peer to unbind, and leaves the socket free for a new
// listener. This simulates the desktop listener startup path: it
// finds a CLI-owned socket, takes it over, and binds fresh.
func TestTakeoverSocketShutsDownLiveDaemon(t *testing.T) {
	ctx := t.Context()

	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-live"), "d.sock")
	done := startControlListener(t, ctx, sock)

	le := logrus.NewEntry(logrus.New())
	if err := TakeoverSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CLI-like daemon did not exit after takeover")
	}

	newLis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("relisten after takeover: %v", err)
	}
	newLis.Close()
	_ = os.Remove(sock)
}

// TestTakeoverSocketRemovesStaleFile asserts that TakeoverSocket
// removes an orphaned socket file when no daemon answers.
func TestTakeoverSocketRemovesStaleFile(t *testing.T) {
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-stale"), "d.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	le := logrus.NewEntry(logrus.New())
	if err := TakeoverSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("expected socket removed; stat err=%v", err)
	}
}

// TestTakeoverSocketNoop asserts that TakeoverSocket succeeds
// silently when nothing is at the socket path.
func TestTakeoverSocketNoop(t *testing.T) {
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-none"), "d.sock")

	le := logrus.NewEntry(logrus.New())
	if err := TakeoverSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}
}

// startControlListener spawns a minimal Unix socket listener that
// registers the daemon-control handler. The shutdown callback closes
// the listener and removes the socket file, matching the desktop
// resource listener controller's Execute teardown.
func startControlListener(t *testing.T, ctx context.Context, sock string) <-chan struct{} {
	t.Helper()

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	serveCtx, serveCancel := context.WithCancel(ctx)
	mux := srpc.NewMux()
	if err := mux.Register(NewHandler(nil, func() {
		serveCancel()
		lis.Close()
	})); err != nil {
		lis.Close()
		t.Fatalf("register: %v", err)
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
	return done
}

// makeShortTakeoverDir returns a short, test-package-local directory
// for Unix sockets; mirrors the helper in the spacewave-cli tests.
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
