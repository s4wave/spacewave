//go:build !js

package control

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// TestTakeoverSocketShutsDownLiveDaemon asserts that the completion
// acknowledgement is not delivered until the old owner has unlinked
// its socket. A replacement can bind immediately, and the old owner's
// later serve-loop exit cannot unlink the replacement.
func TestTakeoverSocketShutsDownLiveDaemon(t *testing.T) {
	ctx := t.Context()

	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-live"), "d.sock")
	done := startControlListener(t, ctx, sock)

	le := logrus.NewEntry(logrus.New())
	if err := TakeoverSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	newLis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("relisten immediately after takeover: %v", err)
	}
	defer newLis.Close()
	assertSocketAccepts(t, sock)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CLI-like daemon did not exit after takeover")
	}
	assertSocketAccepts(t, sock)
}

// TestTakeoverSocketWaitsForHandoffCompletionEvent reproduces the
// legacy ordering that acknowledged before releasing the listener.
// The requester must stay blocked on the socket-path event until the
// old owner completes release.
func TestTakeoverSocketWaitsForHandoffCompletionEvent(t *testing.T) {
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-event"), "d.sock")
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	acknowledged := make(chan struct{})
	release := make(chan struct{})
	mux := srpc.NewMux()
	if err := mux.Register(&ackBeforeReleaseHandler{
		acknowledged: acknowledged,
		release:      release,
		shutdown: func() {
			_ = lis.Close()
		},
	}); err != nil {
		t.Fatalf("register control: %v", err)
	}
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			_ = conn.Close()
			return
		}
		_ = srpc.NewServer(mux).AcceptMuxedConn(ctx, mp)
	}()
	t.Cleanup(func() {
		_ = lis.Close()
	})

	result := make(chan error, 1)
	go func() {
		result <- TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock)
	}()
	select {
	case <-acknowledged:
	case <-time.After(5 * time.Second):
		t.Fatal("old owner did not acknowledge takeover")
	}
	select {
	case err := <-result:
		t.Fatalf("takeover returned before socket release: %v", err)
	default:
	}

	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("takeover after completion event: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("takeover did not observe socket release")
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("replacement listen: %v", err)
	}
	defer replacement.Close()
	assertSocketAccepts(t, sock)
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

// TestTakeoverSocketReclaimsWhenYieldingPeerExitsBeforeCompletion
// asserts that connection closure after yield is treated as owner
// death, not as a permanent handoff wait. The stale path is removed
// and a replacement can serve without a timeout.
func TestTakeoverSocketReclaimsWhenYieldingPeerExitsBeforeCompletion(t *testing.T) {
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-peer-exit"), "d.sock")
	done := startExitBeforeCompletionListener(t, ctx, sock)

	if err := TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock); err != nil {
		t.Fatalf("takeover after peer exit: %v", err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("replacement listen: %v", err)
	}
	defer replacement.Close()
	assertSocketAccepts(t, sock)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("yielding peer did not exit")
	}
}

// TestConcurrentTakeoverRequestsHaveOneWinner asserts that two
// requests admitted by one old owner cannot both bind the socket.
func TestConcurrentTakeoverRequestsHaveOneWinner(t *testing.T) {
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-concurrent"), "d.sock")
	arrived, release := startGatedControlListener(t, ctx, sock)

	type result struct {
		lis *net.UnixListener
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			err := TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock)
			if err != nil {
				results <- result{err: err}
				return
			}
			lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
			results <- result{lis: lis, err: err}
		}()
	}

	for range 2 {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent takeover request did not reach policy")
		}
	}
	close(release)

	var winner *net.UnixListener
	for range 2 {
		res := <-results
		if res.err == nil {
			if winner != nil {
				res.lis.Close()
				winner.Close()
				t.Fatal("two concurrent takeover requests bound the socket")
			}
			winner = res.lis
		}
	}
	if winner == nil {
		t.Fatal("no concurrent takeover request bound the socket")
	}
	defer winner.Close()
	assertSocketAccepts(t, sock)
}

type ackBeforeReleaseHandler struct {
	acknowledged chan<- struct{}
	release      <-chan struct{}
	shutdown     func()
}

func (h *ackBeforeReleaseHandler) GetServiceID() string {
	return ServiceID
}

func (h *ackBeforeReleaseHandler) GetMethodIDs() []string {
	return []string{ShutdownMethodID}
}

func (h *ackBeforeReleaseHandler) InvokeMethod(
	serviceID string,
	methodID string,
	strm srpc.Stream,
) (bool, error) {
	if serviceID != ServiceID || methodID != ShutdownMethodID {
		return false, nil
	}
	if err := strm.MsgRecv(&emptypb.Empty{}); err != nil {
		return true, err
	}
	if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}
	h.acknowledged <- struct{}{}
	select {
	case <-strm.Context().Done():
		return true, strm.Context().Err()
	case <-h.release:
	}
	h.shutdown()
	return true, strm.CloseSend()
}

// startControlListener spawns a minimal Unix socket listener that
// registers the daemon-control handler. UnixListener.Close owns the
// socket unlink, matching the production listener lifecycle.
func startControlListener(t *testing.T, ctx context.Context, sock string) <-chan struct{} {
	t.Helper()

	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
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
		defer lis.Close()
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

func startExitBeforeCompletionListener(
	t *testing.T,
	ctx context.Context,
	sock string,
) <-chan struct{} {
	t.Helper()
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	lis.SetUnlinkOnClose(false)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		mux := srpc.NewMux()
		if err := mux.Register(NewHandler(nil, func() {
			_ = lis.Close()
			_ = conn.Close()
		})); err != nil {
			return
		}
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			_ = conn.Close()
			return
		}
		_ = srpc.NewServer(mux).AcceptMuxedConn(ctx, mp)
	}()
	t.Cleanup(func() {
		_ = lis.Close()
		_ = os.Remove(sock)
	})
	return done
}

func startGatedControlListener(
	t *testing.T,
	ctx context.Context,
	sock string,
) (<-chan struct{}, chan<- struct{}) {
	t.Helper()
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	policy := func(ctx context.Context) error {
		arrived <- struct{}{}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}
	mux := srpc.NewMux()
	if err := mux.Register(NewHandler(policy, func() {
		_ = lis.Close()
	})); err != nil {
		t.Fatalf("register control: %v", err)
	}
	server := srpc.NewServer(mux)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				mp, err := srpc.NewMuxedConn(conn, false, nil)
				if err != nil {
					_ = conn.Close()
					return
				}
				_ = server.AcceptMuxedConn(ctx, mp)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = lis.Close()
	})
	return arrived, release
}

func assertSocketAccepts(t *testing.T, sock string) {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial replacement socket: %v", err)
	}
	_ = conn.Close()
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
