//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

// TestServeDaemonListenerDesktopQuitAckCompletesBeforeDrain verifies that a
// desktop stop claim preserves the requesting RPC reply before client drain.
func TestServeDaemonListenerDesktopQuitAckCompletesBeforeDrain(t *testing.T) {
	// Keep the Unix socket path short on macOS and within this worktree.
	root := filepath.Join("..", "..", "..", ".tmp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "q-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	lis, err := net.Listen("unix", filepath.Join(dir, socketName))
	if err != nil {
		t.Fatal(err)
	}

	// Wire the same listener callback and acknowledgement fence as serve.
	// Cancel serve and shutdown together.
	serveCtx, serveCancel := context.WithCancel(t.Context())
	defer serveCancel()
	shutdownCtx, shutdownCancel := context.WithCancel(serveCtx)
	defer shutdownCancel()

	// Register the control and desktop Quit handlers.
	controlHandler := newDaemonControlHandler(func() {})
	quitHandler := &desktopQuitAckHandler{requestShutdown: func(requester *trackedConn) {
		controlHandler.desktopQuitConn.Store(requester)
		shutdownCancel()
		_ = lis.Close()
	}}
	mux := srpc.NewMux()
	if err := mux.Register(controlHandler); err != nil {
		t.Fatal(err)
	}
	if err := mux.Register(quitHandler); err != nil {
		t.Fatal(err)
	}

	// Serve until the fake desktop Quit handler closes admission.
	finished := make(chan error, 1)
	go func() {
		finished <- serveDaemonListener(serveCtx, serveCancel, lis, srpc.NewServer(mux), controlHandler, shutdownCtx.Done(), nil)
	}()

	// Read the reply after admission closes, then release the requesting socket.
	conn, err := net.Dial("unix", lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if err := client.ExecCall(t.Context(), quitHandler.GetServiceID(), "Quit", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		_ = conn.Close()
		t.Fatalf("desktop Quit reply: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not drain after desktop Quit reply")
	}
}

// startDaemonListener wires a daemon-control handler exactly as runServeCommand
// does: the Shutdown callback closes only the accepting listener and signals
// shutdownCh, never canceling serveCtx. It starts serving and returns the socket
// path plus a wait function that returns the serve result or fails on hang.
func startDaemonListener(t *testing.T) (sock string, wait func() error) {
	return startDaemonListenerWithPolicy(t, listener_control.AutoAllowPolicy)
}

func startDaemonListenerWithPolicy(
	t *testing.T,
	policy listener_control.YieldPolicy,
) (sock string, wait func() error) {
	// Mark the helper.
	t.Helper()

	// Listen on a short Unix socket.
	dir := shortSocketDir(t)
	sock = filepath.Join(dir, socketName)
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Cancel the serve context when the test ends.
	serveCtx, serveCancel := context.WithCancel(t.Context())
	t.Cleanup(serveCancel)

	// Register a control handler that closes admission.
	shutdownCh := make(chan struct{})
	var shutdownOnce sync.Once
	controlHandler := newDaemonControlHandlerWithPolicy(policy, func() {
		shutdownOnce.Do(func() { close(shutdownCh) })
		lis.Close()
	})
	mux := srpc.NewMux()
	if err := mux.Register(controlHandler); err != nil {
		t.Fatalf("register: %v", err)
	}
	srv := srpc.NewServer(mux)

	// Serve the daemon listener and return its wait function.
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, shutdownCh, nil)
	}()
	wait = func() error {
		select {
		case err := <-serveErrCh:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("serve did not return")
			return nil
		}
	}
	return sock, wait
}

// TestServeDaemonListenerShutdownAckCompletesBeforeDrain proves that an
// approved Shutdown request receives its acknowledgement and a clean stream
// close: the requester never observes a reset because the connection lifecycle
// is canceled only after ShutdownComplete.
func TestServeDaemonListenerShutdownAckCompletesBeforeDrain(t *testing.T) {
	// Start a daemon listener.
	sock, wait := startDaemonListener(t)

	// Dial the daemon socket.
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// RequestShutdown reads the acknowledgement and a clean stream close; a
	// reset here is the regressed behavior. The requester then closes its
	// connection, mirroring runStop's deferred close, so the daemon drains.
	if err := listener_control.RequestShutdown(t.Context(), conn); err != nil {
		conn.Close()
		t.Fatalf("shutdown request: %v", err)
	}
	conn.Close()
	if err := wait(); err != nil {
		t.Fatalf("serve: %v", err)
	}
}

// TestServeDaemonListenerWaitsForGrantedRequester proves a stream that reaches
// the handler but withholds its request body cannot strand the actual winner.
func TestServeDaemonListenerWaitsForGrantedRequester(t *testing.T) {
	// Listen on a short Unix socket.
	dir := shortSocketDir(t)
	sock := filepath.Join(dir, socketName)
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Open serve and shutdown state.
	serveCtx, serveCancel := context.WithCancel(t.Context())
	t.Cleanup(serveCancel)
	shutdownCh := make(chan struct{})
	var shutdownOnce sync.Once
	controlHandler := newDaemonControlHandler(func() {
		shutdownOnce.Do(func() { close(shutdownCh) })
		_ = lis.Close()
	})

	// Serve behind a signal invoker.
	entered := make(chan struct{})
	mux := srpc.NewMux(&signalInvoker{
		handler: controlHandler,
		entered: entered,
	})
	srv := srpc.NewServer(mux)
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- serveDaemonListener(
			serveCtx,
			serveCancel,
			lis,
			srv,
			controlHandler,
			shutdownCh,
			nil,
		)
	}()

	// Dial the first requester and open a client.
	firstConn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial first requester: %v", err)
	}
	defer firstConn.Close()
	firstClient, err := srpc.NewClientWithConn(firstConn, true, nil)
	if err != nil {
		t.Fatalf("create first client: %v", err)
	}

	// Open a stalled shutdown stream and wait for the handler.
	firstStream, err := firstClient.NewStream(
		t.Context(),
		listener_control.ServiceID,
		listener_control.ShutdownMethodID,
		nil,
	)
	if err != nil {
		t.Fatalf("open stalled shutdown stream: %v", err)
	}
	defer firstStream.Close()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first shutdown stream did not reach handler")
	}

	// Request shutdown from a second connection.
	secondConn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial second requester: %v", err)
	}
	if err := listener_control.RequestShutdown(t.Context(), secondConn); err != nil {
		secondConn.Close()
		t.Fatalf("second shutdown request: %v", err)
	}
	if err := secondConn.Close(); err != nil {
		t.Fatalf("close second requester: %v", err)
	}

	// Require the serve to drain after the granted requester closes.
	select {
	case err := <-serveErrCh:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not drain after granted requester closed")
	}
}

type signalInvoker struct {
	handler srpc.Invoker
	entered chan struct{}
	once    sync.Once
}

func (i *signalInvoker) InvokeMethod(
	serviceID string,
	methodID string,
	strm srpc.Stream,
) (bool, error) {
	if serviceID == listener_control.ServiceID && methodID == listener_control.ShutdownMethodID {
		i.once.Do(func() { close(i.entered) })
	}
	return i.handler.InvokeMethod(serviceID, methodID, strm)
}

// TestServeDaemonListenerExternalCancelExitsPromptly proves that canceling the
// serve context with an active client stops acceptance, drains the client, and
// returns without hanging.
func TestServeDaemonListenerExternalCancelExitsPromptly(t *testing.T) {
	// Cancel the serve context when the test ends.
	serveCtx, serveCancel := context.WithCancel(t.Context())
	defer serveCancel()

	// Listen on a short Unix socket.
	dir := shortSocketDir(t)
	sock := filepath.Join(dir, socketName)
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Register an empty control handler.
	shutdownCh := make(chan struct{})
	controlHandler := newDaemonControlHandler(func() {})
	mux := srpc.NewMux()
	if err := mux.Register(controlHandler); err != nil {
		t.Fatalf("register: %v", err)
	}
	srv := srpc.NewServer(mux)

	// Serve the daemon listener.
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, shutdownCh, nil)
	}()

	// Dial the daemon socket.
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Cancel serve and require it to exit.
	serveCancel()
	select {
	case err := <-serveErrCh:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external cancel did not exit")
	}
}

// TestServeDaemonListenerAcceptErrorPropagates proves that a genuine accept
// error that is neither an approved shutdown nor an external cancellation is
// returned to the caller instead of being suppressed.
func TestServeDaemonListenerAcceptErrorPropagates(t *testing.T) {
	// Cancel the serve context when the test ends.
	serveCtx, serveCancel := context.WithCancel(t.Context())
	defer serveCancel()

	// Build a listener whose accept always fails.
	sentinel := stderrors.New("accept boom")
	lis := &fakeAcceptListener{acceptErr: sentinel}
	shutdownCh := make(chan struct{})
	controlHandler := newDaemonControlHandler(func() {})
	srv := srpc.NewServer(srpc.NewMux())

	// Require the sentinel accept error.
	err := serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, shutdownCh, nil)
	if !stderrors.Is(err, sentinel) {
		t.Fatalf("expected sentinel accept error, got %v", err)
	}
}

// fakeAcceptListener is a net.Listener whose Accept always fails with a fixed
// error, used to exercise the accept-error exit path.
type fakeAcceptListener struct {
	acceptErr error
}

func (l *fakeAcceptListener) Accept() (net.Conn, error) { return nil, l.acceptErr }
func (l *fakeAcceptListener) Close() error              { return nil }
func (l *fakeAcceptListener) Addr() net.Addr            { return &net.UnixAddr{Name: "fake", Net: "unix"} }
