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

	// Leave a real Unix socket path behind after its listener exits.
	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	lis.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := lis.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	// Takeover may remove the orphan socket and make the path available.
	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("expected socket removed; stat err=%v", err)
	}
}

// TestTakeoverDaemonSocketPreservesNonSocket asserts takeover refuses to
// remove user data occupying the socket path.
func TestTakeoverDaemonSocketPreservesNonSocket(t *testing.T) {
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-file"), "stale.sock")
	const contents = "user data"

	// Put an ordinary file at the requested socket path.
	if err := os.WriteFile(sock, []byte(contents), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Takeover must refuse the path without changing its contents.
	le := logrus.NewEntry(logrus.New())
	if err := takeoverDaemonSocket(ctx, le, sock); err == nil || !strings.Contains(err.Error(), "is not a socket") {
		t.Fatalf("expected non-socket refusal, got %v", err)
	}
	dat, err := os.ReadFile(sock)
	if err != nil {
		t.Fatalf("read file after takeover: %v", err)
	}
	if string(dat) != contents {
		t.Fatalf("file contents changed after takeover: %q", dat)
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
	return startDesktopLikeListenerWithShutdown(t, ctx, sock, nil)
}

func startDesktopLikeListenerWithShutdown(
	t *testing.T,
	ctx context.Context,
	sock string,
	beforeShutdown func(),
) *desktopLikeListener {
	t.Helper()

	lis, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	serveCtx, serveCancel := context.WithCancel(ctx)
	mux := srpc.NewMux()
	if err := mux.Register(newDaemonControlHandler(func() {
		if beforeShutdown != nil {
			beforeShutdown()
		}
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
