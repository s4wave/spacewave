package transport_controller

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

type mountedStreamHandlerController struct {
	handler link.MountedStreamHandler
}

func (c *mountedStreamHandlerController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(link.HandleMountedStream); !ok {
		return nil, nil
	}
	return directive.Resolvers(
		directive.NewValueResolver([]link.MountedStreamHandler{c.handler}),
	), nil
}

func (c *mountedStreamHandlerController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/mounted-stream-handler", controller.MustParseVersion("0.0.1"), "test mounted stream handler")
}

func (*mountedStreamHandlerController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (*mountedStreamHandlerController) Close() error {
	return nil
}

type blockingMountedStreamHandler struct {
	started  chan struct{}
	returned chan error
	refs     atomic.Int32
}

func (h *blockingMountedStreamHandler) HandleMountedStream(_ context.Context, ms link.MountedStream) error {
	// Retain the mounted handler reference while the stream read is active.
	h.refs.Add(1)
	defer h.refs.Add(-1)

	// Report the mounted handler start and the eventual stream read result.
	close(h.started)
	_, err := ms.GetStream().Read(make([]byte, 1))
	h.returned <- err
	return err
}

type mountedStreamTestLink struct {
	localPeer  peer.ID
	remotePeer peer.ID
}

func (l *mountedStreamTestLink) GetUUID() uint64 {
	return 1
}

func (l *mountedStreamTestLink) GetTransportUUID() uint64 {
	return 2
}

func (l *mountedStreamTestLink) OpenStream(stream.OpenOpts) (stream.Stream, error) {
	return nil, errors.New("not implemented")
}

func (l *mountedStreamTestLink) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	return nil, stream.OpenOpts{}, errors.New("not implemented")
}

func (l *mountedStreamTestLink) GetRemotePeer() peer.ID {
	return l.remotePeer
}

func (l *mountedStreamTestLink) GetLocalPeer() peer.ID {
	return l.localPeer
}

func (l *mountedStreamTestLink) GetRemoteTransportUUID() uint64 {
	return 3
}

func (*mountedStreamTestLink) Close() error {
	return nil
}

type blockingMountedStream struct {
	data      []byte
	dataPos   int
	closed    chan struct{}
	closeOnce sync.Once
	closeCnt  atomic.Int32
}

func newBlockingMountedStream(protocolID protocol.ID) *blockingMountedStream {
	return &blockingMountedStream{
		data:   marshalStreamEstablishHeader(NewStreamEstablish(protocolID)),
		closed: make(chan struct{}),
	}
}

func (s *blockingMountedStream) Read(buf []byte) (int, error) {
	if s.dataPos < len(s.data) {
		n := copy(buf, s.data[s.dataPos:])
		s.dataPos += n
		return n, nil
	}
	<-s.closed
	return 0, io.ErrClosedPipe
}

func (s *blockingMountedStream) Write(buf []byte) (int, error) {
	return len(buf), nil
}

func (*blockingMountedStream) SetReadDeadline(time.Time) error {
	return nil
}

func (*blockingMountedStream) SetWriteDeadline(time.Time) error {
	return nil
}

func (*blockingMountedStream) SetDeadline(time.Time) error {
	return nil
}

func (s *blockingMountedStream) Close() error {
	s.closeCnt.Add(1)
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

// TestHandleIncomingStreamClosesOnContextCancel proves a mounted stream closes
// when its link context is canceled, unblocking a context-unaware handler read.
func TestHandleIncomingStreamClosesOnContextCancel(t *testing.T) {
	// Create a cancellable transport test context.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a transport testbed without peer or echo controllers.
	log := logrus.New()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.TestbedOpts{
		NoPeer: true,
		NoEcho: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register a mounted stream handler that blocks while reading.
	handler := &blockingMountedStreamHandler{
		started:  make(chan struct{}),
		returned: make(chan error, 1),
	}
	handlerCtrl := &mountedStreamHandlerController{handler: handler}
	rel, err := tb.Bus.AddController(ctx, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	// Prepare the transport controller and incoming mounted stream.
	localPeer := peer.ID("local")
	remotePeer := peer.ID("remote")
	protocolID := protocol.ID("test/mounted")
	ctrl := &Controller{
		le:            logrus.NewEntry(log),
		bus:           tb.Bus,
		info:          controller.NewInfo("test/transport", controller.MustParseVersion("0.0.1"), "test transport"),
		links:         make(map[uint64]*establishedLink),
		linksByPeerID: make(map[peer.ID][]*establishedLink),
	}
	strm := newBlockingMountedStream(protocolID)
	lnk := &mountedStreamTestLink{localPeer: localPeer, remotePeer: remotePeer}

	// Run incoming stream handling under a cancellable link context.
	rctx, cancelLink := context.WithCancel(ctx)
	handleDone := make(chan struct{})
	go func() {
		ctrl.HandleIncomingStream(rctx, nil, lnk, strm, stream.OpenOpts{})
		close(handleDone)
	}()

	// Wait for the mounted handler to begin its blocking read.
	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mounted stream handler")
	}

	// Cancel the link context to close the mounted stream.
	cancelLink()

	// Verify stream closure unblocks the mounted handler with a closed-pipe error.
	select {
	case err := <-handler.returned:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("handler returned %v, want closed pipe", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mounted stream handler to return")
	}

	// Wait for incoming stream handling to finish after link cancellation.
	select {
	case <-handleDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for incoming stream handler")
	}

	// Verify the mounted handler releases its reference and the stream closes.
	if refs := handler.refs.Load(); refs != 0 {
		t.Fatalf("mounted stream handler refs = %d, want 0", refs)
	}
	if closes := strm.closeCnt.Load(); closes < 2 {
		t.Fatalf("stream close count = %d, want at least 2 for watchdog and handler cleanup", closes)
	}
}

var (
	_ link.MountedStreamHandler = (*blockingMountedStreamHandler)(nil)
	_ stream.Stream             = (*blockingMountedStream)(nil)
)

type mountedSignalingHandler struct {
	sessions atomic.Int32
	refs     atomic.Int32
	events   chan string
}

func (h *mountedSignalingHandler) HandleMountedStream(_ context.Context, ms link.MountedStream) error {
	// Track the signaling session and retain its active handler reference.
	session := h.sessions.Add(1)
	h.refs.Add(1)
	defer h.refs.Add(-1)

	// Report the first signaling session lifetime around its blocking stream read.
	if session == 1 {
		h.events <- "Opened"
		_, err := ms.GetStream().Read(make([]byte, 1))
		h.events <- "Closed"
		return err
	}

	// Report the replacement signaling session opening to the test.
	h.events <- "Opened"
	return nil
}

// TestMountedSignalingStreamReacceptAfterContextCancel proves a canceled
// mounted signaling stream can be replaced by a fresh opened session.
func TestMountedSignalingStreamReacceptAfterContextCancel(t *testing.T) {
	// Create a cancellable signaling stream test context.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a signaling testbed without peer or echo controllers.
	log := logrus.New()
	tbs, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.TestbedOpts{
		NoPeer: true,
		NoEcho: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tbs.Release)

	// Register the signaling handler that reports opened and closed sessions.
	handler := &mountedSignalingHandler{events: make(chan string, 4)}
	handlerCtrl := &mountedStreamHandlerController{handler: handler}
	rel, err := tbs.Bus.AddController(ctx, handlerCtrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	// Prepare the transport controller and peer link for both signaling sessions.
	localPeer := peer.ID("local")
	remotePeer := peer.ID("remote")
	protocolID := protocol.ID("test/signaling")
	ctrl := &Controller{
		le:            logrus.NewEntry(log),
		bus:           tbs.Bus,
		info:          controller.NewInfo("test/transport", controller.MustParseVersion("0.0.1"), "test transport"),
		links:         make(map[uint64]*establishedLink),
		linksByPeerID: make(map[peer.ID][]*establishedLink),
	}
	lnk := &mountedStreamTestLink{localPeer: localPeer, remotePeer: remotePeer}

	// Start the first signaling stream under a cancellable link context.
	firstStream := newBlockingMountedStream(protocolID)
	firstCtx, cancelFirst := context.WithCancel(ctx)
	firstDone := make(chan struct{})
	go func() {
		ctrl.HandleIncomingStream(firstCtx, nil, lnk, firstStream, stream.OpenOpts{})
		close(firstDone)
	}()

	// Verify the first signaling session opens before cancelling its context.
	if event := recvMountedSignalingEvent(t, handler.events); event != "Opened" {
		t.Fatalf("first session event = %q, want Opened", event)
	}
	cancelFirst()

	// Verify the canceled signaling session closes and releases its handler reference.
	if event := recvMountedSignalingEvent(t, handler.events); event != "Closed" {
		t.Fatalf("first session event = %q, want Closed", event)
	}
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first signaling stream")
	}
	if refs := handler.refs.Load(); refs != 0 {
		t.Fatalf("signaling handler refs after first session = %d, want 0", refs)
	}

	// Start a replacement signaling stream on the same peer link.
	secondStream := newBlockingMountedStream(protocolID)
	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondDone := make(chan struct{})
	go func() {
		ctrl.HandleIncomingStream(secondCtx, nil, lnk, secondStream, stream.OpenOpts{})
		close(secondDone)
	}()

	// Verify the replacement signaling session opens before cancelling its context.
	if event := recvMountedSignalingEvent(t, handler.events); event != "Opened" {
		t.Fatalf("second session event = %q, want Opened", event)
	}
	cancelSecond()

	// Verify the replacement stream finishes, closes, and releases its handler reference.
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second signaling stream")
	}
	select {
	case <-secondStream.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second signaling stream close")
	}
	if refs := handler.refs.Load(); refs != 0 {
		t.Fatalf("signaling handler refs after second session = %d, want 0", refs)
	}
}

func recvMountedSignalingEvent(t *testing.T, events <-chan string) string {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signaling event")
		return ""
	}
}
