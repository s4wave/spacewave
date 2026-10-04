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
	// Use the test lifetime for the daemon takeover.
	ctx := t.Context()

	// Start a live daemon listener at the takeover socket path.
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-live"), "d.sock")
	done := startControlListener(t, ctx, sock)

	// Request takeover of the live daemon socket.
	le := logrus.NewEntry(logrus.New())
	handedOff, err := TakeoverSocket(ctx, le, sock)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if !handedOff {
		t.Fatal("takeover did not report the live daemon's handoff")
	}

	// Bind a replacement listener immediately after the daemon yields.
	newLis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("relisten immediately after takeover: %v", err)
	}
	defer newLis.Close()
	assertSocketAccepts(t, sock)

	// Verify the old daemon exits without unlinking the replacement socket.
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
	// Bind the old daemon socket for an acknowledgement-before-release handoff.
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-event"), "d.sock")
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Gate listener release after the daemon acknowledges the shutdown request.
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

	// Serve the old daemon until its release gate closes.
	go func() {
		// Accept and serve the old daemon control connection.
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

	// Release the old daemon listener when the test ends.
	t.Cleanup(func() {
		_ = lis.Close()
	})

	// Start the takeover request and require it to wait after acknowledgement.
	result := make(chan error, 1)
	go func() {
		_, err := TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock)
		result <- err
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

	// Release the old listener and require the takeover to complete.
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("takeover after completion event: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("takeover did not observe socket release")
	}

	// Bind a replacement listener after the socket release event.
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
	// Leave an orphaned socket at the daemon path.
	ctx := context.Background()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-stale"), "d.sock")
	createStaleTakeoverSocket(t, sock)

	// Request takeover of the orphaned daemon socket.
	le := logrus.NewEntry(logrus.New())
	handedOff, err := TakeoverSocket(ctx, le, sock)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if handedOff {
		t.Fatal("takeover reported a handoff from a stale socket")
	}

	// Require the orphaned daemon socket path to be removed.
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
	if _, err := TakeoverSocket(ctx, le, sock); err != nil {
		t.Fatalf("takeover: %v", err)
	}
}

func TestEnsureSocketAvailableRefusesLiveListener(t *testing.T) {
	// Bind a live listener that must be protected from implicit takeover.
	sock := filepath.Join(makeShortTakeoverDir(t, "ensure-live"), "d.sock")
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	// Require socket availability to report the live listener refusal.
	err = EnsureSocketAvailable(t.Context(), logrus.NewEntry(logrus.New()), sock)
	if err == nil {
		t.Fatal("expected live socket refusal")
	}
	if want := "daemon socket " + sock + " is already in use"; err.Error() != want {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureSocketAvailableRemovesStaleFile(t *testing.T) {
	sock := filepath.Join(makeShortTakeoverDir(t, "ensure-stale"), "d.sock")
	createStaleTakeoverSocket(t, sock)

	if err := EnsureSocketAvailable(t.Context(), logrus.NewEntry(logrus.New()), sock); err != nil {
		t.Fatalf("ensure socket available: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("expected stale socket removed; stat err=%v", err)
	}
}

// TestTakeoverSocketReclaimsWhenYieldingPeerExitsBeforeCompletion
// asserts that connection closure after yield is treated as owner
// death, not as a permanent handoff wait. The stale path is removed
// and a replacement can serve without a timeout.
func TestTakeoverSocketReclaimsWhenYieldingPeerExitsBeforeCompletion(t *testing.T) {
	// Start a daemon that exits before acknowledging completed shutdown.
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-peer-exit"), "d.sock")
	done := startExitBeforeCompletionListener(t, ctx, sock)

	// Reclaim the yielded socket and bind a replacement listener.
	handedOff, err := TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock)
	if err != nil {
		t.Fatalf("takeover after peer exit: %v", err)
	}
	if handedOff {
		t.Fatal("takeover reported a handoff from a peer that never acknowledged")
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("replacement listen: %v", err)
	}
	defer replacement.Close()
	assertSocketAccepts(t, sock)

	// Require the yielding peer to finish its serving goroutine.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("yielding peer did not exit")
	}
}

// TestConcurrentTakeoverRequestsHaveOneWinner asserts that two
// requests admitted by one old owner cannot both bind the socket.
func TestConcurrentTakeoverRequestsHaveOneWinner(t *testing.T) {
	// Start a daemon whose policy gates two simultaneous takeover requests.
	ctx := t.Context()
	sock := filepath.Join(makeShortTakeoverDir(t, "takeover-concurrent"), "d.sock")
	arrived, release := startGatedControlListener(t, ctx, sock)

	// Launch two requesters that compete to bind the released socket.
	type result struct {
		lis *net.UnixListener
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			// Request takeover and report any denial before binding a replacement.
			_, err := TakeoverSocket(ctx, logrus.NewEntry(logrus.New()), sock)
			if err != nil {
				results <- result{err: err}
				return
			}

			// Bind the replacement socket after this requester completes takeover.
			lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
			results <- result{lis: lis, err: err}
		}()
	}

	// Wait for both requesters to reach the policy before releasing the daemon.
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent takeover request did not reach policy")
		}
	}
	close(release)

	// Require exactly one requester to retain the replacement listener.
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
	// Acknowledge the shutdown request before releasing the old listener.
	if serviceID != ServiceID || methodID != ShutdownMethodID {
		return false, nil
	}
	if err := strm.MsgRecv(&emptypb.Empty{}); err != nil {
		return true, err
	}
	if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}

	// Hold the acknowledged request until the test releases the daemon listener.
	h.acknowledged <- struct{}{}
	select {
	case <-strm.Context().Done():
		return true, strm.Context().Err()
	case <-h.release:
	}

	// Release the old listener before completing the response stream.
	h.shutdown()
	return true, strm.CloseSend()
}

// startControlListener spawns a minimal Unix socket listener that
// registers the daemon-control handler. UnixListener.Close owns the
// socket unlink, matching the production listener lifecycle.
func startControlListener(t *testing.T, ctx context.Context, sock string) <-chan struct{} {
	// Attribute daemon listener setup failures to the calling test.
	t.Helper()

	// Bind the daemon control listener at the requested socket path.
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Register a shutdown handler that releases the socket and stops serving
	// once the requester has its acknowledgement, as a daemon drains.
	serveCtx, serveCancel := context.WithCancel(ctx)
	h := NewHandler(nil, func() { lis.Close() })
	go func() {
		select {
		case <-h.ShutdownComplete():
			serveCancel()
		case <-serveCtx.Done():
		}
	}()
	mux := srpc.NewMux()
	if err := mux.Register(h); err != nil {
		lis.Close()
		t.Fatalf("register: %v", err)
	}

	// Serve daemon control connections until shutdown closes the listener.
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

	// Release the daemon context and listener when the test ends.
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
	// Bind a daemon listener that leaves its socket path behind on exit.
	t.Helper()
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	lis.SetUnlinkOnClose(false)

	// Serve one daemon connection that closes before shutdown completion.
	done := make(chan struct{})
	go func() {
		// Accept the requester and register the daemon shutdown handler.
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
	// Bind a daemon listener for the gated takeover requests.
	t.Helper()
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Gate both takeover policy invocations before permitting shutdown.
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

	// Register the gated shutdown policy on the daemon control mux.
	mux := srpc.NewMux()
	if err := mux.Register(NewHandler(policy, func() {
		_ = lis.Close()
	})); err != nil {
		t.Fatalf("register control: %v", err)
	}

	// Serve each concurrent requester through the gated daemon handler.
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
func makeShortTakeoverDir(t *testing.T, _ string) string {
	// Choose a short socket directory beneath the test state root.
	t.Helper()
	root := os.Getenv("SPACEWAVE_TEST_STATE_ROOT")
	if root == "" {
		root = "../../../../.tmp"
	}

	// Resolve and create the root directory for short daemon socket paths.
	tmpRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	// Allocate a socket directory that is removed when the test ends.
	dir, err := os.MkdirTemp(tmpRoot, "tk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// createStaleTakeoverSocket leaves a real unbound socket, not an ordinary file.
func createStaleTakeoverSocket(t *testing.T, path string) {
	// Create an orphaned socket whose file survives listener closure.
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureSocketAvailablePreservesNonSocket prevents arbitrary file removal
// when a configured socket path actually names an ordinary file.
func TestEnsureSocketAvailablePreservesNonSocket(t *testing.T) {
	path := filepath.Join(makeShortTakeoverDir(t, "regular"), "d.sock")
	if err := os.WriteFile(path, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSocketAvailable(t.Context(), nil, path); err == nil {
		t.Fatal("ordinary file was treated as a stale socket")
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "preserved" {
		t.Fatalf("ordinary file changed: %q, %v", contents, err)
	}
}
