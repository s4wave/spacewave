package dex_solicit

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

func TestControllerSamePeerReplacementWakesOldPendingAndKeepsNewSession(t *testing.T) {
	// Keep replacement sessions within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Prepare the controller and block requested from the replaced peer.
	c := newTestDexSolicitController()
	remote := peer.ID("peer-a")
	ref := testDexBlockRef(t, "replace")

	// Register the first peer session and retain its remote endpoint.
	firstMS, firstRemote, firstCleanup := newTestDexMountedStreamPair(remote)
	defer firstCleanup()
	c.handleSolicitedStream(ctx, link_solicit.NewSolicitMountedStream(firstMS))
	first := waitTestDexSession(t, c, remote)

	// Capture the block request sent over the first session.
	remoteReq := make(chan *DexMessage, 1)
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := firstRemote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		remoteReq <- &req
	}()

	// Request a block whose response will be interrupted by replacement.
	requestDone := make(chan error, 1)
	go func() {
		_, err := first.requestBlock(ctx, ref, 0)
		requestDone <- err
	}()

	// Verify the old session assigned a request identity.
	req := recvTestDexValue(t, remoteReq, "old session request")
	if req.GetRequestId() == 0 {
		t.Fatal("request id was not assigned")
	}

	// Replace the peer session and verify its identity changed.
	secondMS, secondRemote, secondCleanup := newTestDexMountedStreamPair(remote)
	defer secondCleanup()
	c.handleSolicitedStream(ctx, link_solicit.NewSolicitMountedStream(secondMS))
	second := waitTestDexSession(t, c, remote)
	if second == first {
		t.Fatal("replacement kept old session")
	}

	// Verify replacement wakes the old request without a remote read failure.
	err := recvTestDexValue(t, requestDone, "old pending request result")
	if err == nil || !strings.Contains(err.Error(), "session closed") {
		t.Fatalf("old request err = %v, want session closed", err)
	}
	select {
	case err := <-remoteErr:
		t.Fatalf("old remote read failed: %v", err)
	default:
	}

	// Verify stale session cleanup preserves the replacement.
	if err := first.waitExited(ctx); err != nil {
		t.Fatal("old session exit:", err)
	}
	if current := getTestDexSession(c, remote); current != second {
		t.Fatalf("stale cleanup removed replacement session: got %p, want %p", current, second)
	}
	_ = secondRemote.Close()
}

func TestPeerSessionCloseWakesPendingRequestsWithoutRunLoop(t *testing.T) {
	// Keep the pending request within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Prepare a peer session whose receive loop remains stopped.
	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "close")
	sess, remote, cleanup := newTestPeerSessionPair(c, peer.ID("close-peer"))
	defer cleanup()

	// Capture the request sent before the peer session closes.
	remoteReq := make(chan *DexMessage, 1)
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		remoteReq <- &req
	}()

	// Start a pending block request on the stopped session.
	requestDone := make(chan error, 1)
	go func() {
		_, err := sess.requestBlock(ctx, ref, 0)
		requestDone <- err
	}()

	// Verify the pending request received a request identity.
	req := recvTestDexValue(t, remoteReq, "request before close")
	if req.GetRequestId() == 0 {
		t.Fatal("request id was not assigned")
	}

	// Close the session and verify its pending request fails.
	sess.close()
	err := recvTestDexValue(t, requestDone, "pending request close result")
	if err == nil || !strings.Contains(err.Error(), "session closed") {
		t.Fatalf("close err = %v, want session closed", err)
	}
	select {
	case err := <-remoteErr:
		t.Fatalf("remote read failed before close: %v", err)
	default:
	}
}

func TestPeerSessionRejectsMismatchedBlockData(t *testing.T) {
	// Keep the corrupt peer session within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start a peer session for the corrupt response.
	c := newTestDexSolicitController()
	sess, remote, cleanup := newTestPeerSessionPair(c, peer.ID("corrupt-peer"))
	defer cleanup()
	sess.start(ctx)

	// Answer the peer request with bytes that mismatch its block reference.
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		remoteErr <- remote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       []byte("corrupt"),
		})
	}()

	// Verify corrupt block bytes fail verification without returning a response.
	resp, err := sess.requestBlock(ctx, testDexBlockRef(t, "expected"), 0)
	if err == nil {
		t.Fatal("mismatched block data returned no error")
	}
	if resp != nil {
		t.Fatalf("mismatched block data returned a response: %q", resp.GetData())
	}
	if remoteErr := recvTestDexValue(t, remoteErr, "corrupt block response"); remoteErr != nil {
		t.Fatal(remoteErr)
	}
}

func TestPeerBlockFanoutCancelsLosersAfterFirstSuccess(t *testing.T) {
	// Keep the fanout sessions within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start fast and slow peers for the same block request.
	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "fast-data")
	fast, fastRemote, fastCleanup := newTestPeerSessionPair(c, peer.ID("fast"))
	defer fastCleanup()
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	fast.start(ctx)
	slow.start(ctx)

	// Observe the slow peer receiving its fanout request and then the cancel
	// that withdraws it.
	slowReceived := make(chan struct{})
	slowReq := make(chan *DexMessage, 1)
	slowCancel := make(chan *DexMessage, 1)
	go func() {
		// Capture the request and release the fast responder.
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err != nil {
			return
		}
		slowReq <- &req
		close(slowReceived)

		// Capture the next message, which withdraws the request.
		var msg DexMessage
		if err := slowRemote.RecvMsg(&msg); err == nil {
			slowCancel <- &msg
		}
	}()

	// Make the fast peer respond after both peers receive their requests.
	fastErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := fastRemote.RecvMsg(&req); err != nil {
			fastErr <- err
			return
		}
		select {
		case <-slowReceived:
		case <-ctx.Done():
			fastErr <- ctx.Err()
			return
		}
		fastErr <- fastRemote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       []byte("fast-data"),
		})
	}()

	// Verify the first successful fanout response cancels the slow request.
	found, err := peerBlockFanout{sessions: []*peerSession{slow, fast}, ref: ref}.run(ctx)
	if err != nil || found == nil {
		t.Fatalf("fanout did not return first successful response: %v", err)
	}
	if string(found.GetData()) != "fast-data" {
		t.Fatalf("data = %q, want fast-data", found.GetData())
	}
	if err := recvTestDexValue(t, fastErr, "fast responder result"); err != nil {
		t.Fatalf("fast responder: %v", err)
	}
	waitTestDexCondition(t, "slow pending request to clear after first success", func() bool {
		return testDexPendingLen(slow) == 0
	})

	// Verify the slow peer was told to stop serving the withdrawn request.
	req := recvTestDexValue(t, slowReq, "slow request")
	msg := recvTestDexValue(t, slowCancel, "slow cancel")
	if !msg.GetCancel() || msg.GetRequestId() != req.GetRequestId() {
		t.Fatalf("slow peer got %v, want cancel of request %d", msg, req.GetRequestId())
	}
}

func TestPeerBlockFanoutDeadlineClearsPendingRequests(t *testing.T) {
	// Bound the caller lifetime for the unanswered fanout request.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	// Start a slow peer for the caller deadline test.
	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "deadline")
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	slow.start(ctx)

	// Observe the slow peer receiving the unanswered request.
	received := make(chan struct{})
	go func() {
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err == nil {
			close(received)
		}
	}()

	// Verify the caller deadline ends fanout with an error rather than a miss,
	// and clears its pending request.
	found, err := peerBlockFanout{sessions: []*peerSession{slow}, ref: ref}.run(ctx)
	if found != nil {
		t.Fatalf("fanout returned data after caller deadline: %q", found.GetData())
	}
	if err == nil {
		t.Fatal("fanout reported an unanswered request as a miss")
	}
	recvTestDexValue(t, received, "deadline request")
	waitTestDexCondition(t, "slow pending request to clear after caller deadline", func() bool {
		return testDexPendingLen(slow) == 0
	})
}

func TestLookupResolverCompletesDemandWithoutPeers(t *testing.T) {
	handler := &testDexResolverHandler{}
	resolver := &lookupResolver{
		c:   newTestDexSolicitController(),
		ref: testDexBlockRef(t, "no-peer"),
	}

	if err := resolver.Resolve(t.Context(), handler); err != nil {
		t.Fatal(err)
	}
	assertTestDexNotFoundValue(t, handler)
}

func TestLookupResolverCompletesCurrentPeerMissOnce(t *testing.T) {
	// Keep the missing peer session within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Register the current peer session for the missing block demand.
	c := newTestDexSolicitController()
	remoteID := peer.ID("missing-peer")
	sess, remote, cleanup := newTestPeerSessionPair(c, remoteID)
	defer cleanup()
	sess.start(ctx)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions[remoteID.String()] = sess
		broadcast()
	})

	// Answer the current peer request with a block miss.
	requests := make(chan *DexMessage, 1)
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		requests <- &req
		remoteErr <- remote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
		})
	}()

	// Resolve one missing block while preserving an unrelated reference.
	requested := testDexBlockRef(t, "missing")
	untouched := testDexBlockRef(t, "untouched")
	handler := &testDexResolverHandler{}
	resolved := make(chan error, 1)
	go func() {
		resolved <- (&lookupResolver{
			c:   c,
			ref: requested,
		}).Resolve(ctx, handler)
	}()

	// Verify the resolver requested only the missing block and completed once.
	req := recvTestDexValue(t, requests, "single missing block request")
	if req.GetRequestId() == 0 {
		t.Fatal("request id was not assigned")
	}
	if !req.GetRef().EqualsRef(requested) {
		t.Fatalf("requested ref = %s, want %s", req.GetRef().MarshalString(), requested.MarshalString())
	}
	if req.GetRef().EqualsRef(untouched) {
		t.Fatal("untouched ref was requested")
	}
	if err := recvTestDexValue(t, remoteErr, "missing block response"); err != nil {
		t.Fatal(err)
	}
	if err := recvTestDexValue(t, resolved, "missing block demand completion"); err != nil {
		t.Fatal(err)
	}
	assertTestDexNotFoundValue(t, handler)

	// Verify a link state change does not replay the completed block request.
	extraRequests := make(chan *DexMessage, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err == nil {
			extraRequests <- &req
		}
	}()
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	assertNoTestDexValue(t, extraRequests, "replayed block request after link-state change")
}

func TestLookupResolverReturnsPeerDataWithoutWritingStorage(t *testing.T) {
	// Keep the data peer session within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Register the peer session that supplies the requested block.
	c := newTestDexSolicitController()
	remoteID := peer.ID("data-peer")
	sess, remote, cleanup := newTestPeerSessionPair(c, remoteID)
	defer cleanup()
	sess.start(ctx)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions[remoteID.String()] = sess
		broadcast()
	})

	// Answer the peer request with verified block bytes.
	want := []byte("peer-data")
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		remoteErr <- remote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       want,
		})
	}()

	// Verify the resolver publishes one successful value with the peer bytes.
	handler := &testDexResolverHandler{}
	if err := (&lookupResolver{
		c:   c,
		ref: testDexBlockRef(t, string(want)),
	}).Resolve(ctx, handler); err != nil {
		t.Fatal(err)
	}
	if err := recvTestDexValue(t, remoteErr, "found block response"); err != nil {
		t.Fatal(err)
	}
	if len(handler.values) != 1 {
		t.Fatalf("values = %d, want 1", len(handler.values))
	}
	value, ok := handler.values[0].(dex.LookupBlockFromNetworkValue)
	if !ok {
		t.Fatalf("value type = %T, want LookupBlockFromNetworkValue", handler.values[0])
	}
	if string(value.GetData()) != string(want) {
		t.Fatalf("data = %q, want %q", value.GetData(), want)
	}
	if err := value.GetError(); err != nil {
		t.Fatal(err)
	}
}

// TestStoreReadWaitsForPeerSession proves a read on a settled solicitation
// with no peer waits for a peer session instead of reporting a miss nobody was
// asked about, while an existence check answers at once.
func TestStoreReadWaitsForPeerSession(t *testing.T) {
	// Keep the peer session within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Check existence with no peer connected.
	c := newTestDexSolicitController()
	store := NewStore(c)
	want := []byte("late-peer-data")
	ref := testDexBlockRef(t, string(want))
	exists, err := store.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("existence check found a block with no peer connected")
	}

	// Start a read, which must wait while no peer is connected.
	type readResult struct {
		data  []byte
		found bool
		err   error
	}
	reads := make(chan readResult, 1)
	go func() {
		data, found, err := store.GetBlock(ctx, ref)
		reads <- readResult{data: data, found: found, err: err}
	}()
	assertNoTestDexValue(t, reads, "read before a peer connected")

	// Connect a peer that serves the block.
	remoteID := peer.ID("late-peer")
	sess, remote, cleanup := newTestPeerSessionPair(c, remoteID)
	defer cleanup()
	sess.start(ctx)
	remoteErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			remoteErr <- err
			return
		}
		remoteErr <- remote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       want,
		})
	}()
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions[remoteID.String()] = sess
		broadcast()
	})

	// Verify the waiting read returns the peer's block.
	if err := recvTestDexValue(t, remoteErr, "late peer response"); err != nil {
		t.Fatal(err)
	}
	res := recvTestDexValue(t, reads, "read after the peer connected")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !res.found || string(res.data) != string(want) {
		t.Fatalf("read = %q found=%v, want %q", res.data, res.found, want)
	}
}

func TestControllerForwardToPeersExcludesOrigin(t *testing.T) {
	// Keep the origin peer session within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start an origin peer session for the forwarding exclusion test.
	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "forward")
	origin, originRemote, originCleanup := newTestPeerSessionPair(c, peer.ID("origin"))
	defer originCleanup()
	origin.start(ctx)

	// Register the origin session as the sole forwarding candidate.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions["origin"] = origin
		broadcast()
	})

	// Prepare an origin response to detect an incorrectly forwarded request.
	originErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := originRemote.RecvMsg(&req); err != nil {
			originErr <- err
			return
		}
		originErr <- originRemote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       []byte("origin-data"),
		})
	}()

	// Verify forwarding excludes the originating peer session.
	if found, err := c.forwardToPeers(ctx, ref, 0, origin); found != nil || err != nil {
		t.Fatalf("forwardToPeers used excluded origin session: %v", err)
	}
	assertNoTestDexValue(t, originErr, "origin request")
}

func TestControllerForwardToPeersCancelsLosersAfterFirstSuccess(t *testing.T) {
	// Keep the forwarding peer sessions within the test lifetime.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Start fast and slow peers for the forwarded block request.
	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "forward-data")
	fast, fastRemote, fastCleanup := newTestPeerSessionPair(c, peer.ID("fast"))
	defer fastCleanup()
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	fast.start(ctx)
	slow.start(ctx)

	// Register fast and slow sessions as forwarding candidates.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions["fast"] = fast
		c.sessions["slow"] = slow
		broadcast()
	})

	// Observe the slow peer receiving its forwarded request.
	slowReceived := make(chan struct{})
	go func() {
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err == nil {
			close(slowReceived)
		}
	}()

	// Make the fast forwarding peer answer after the slow peer receives its request.
	fastErr := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := fastRemote.RecvMsg(&req); err != nil {
			fastErr <- err
			return
		}
		select {
		case <-slowReceived:
		case <-ctx.Done():
			fastErr <- ctx.Err()
			return
		}
		fastErr <- fastRemote.SendMsg(&DexMessage{
			RequestId:  req.GetRequestId(),
			IsResponse: true,
			Found:      true,
			Data:       []byte("forward-data"),
		})
	}()

	// Verify forwarded success cancels the slow peer request.
	found, err := c.forwardToPeers(ctx, ref, 0, nil)
	if err != nil || found == nil {
		t.Fatalf("forwardToPeers did not return first successful response: %v", err)
	}
	if string(found.GetData()) != "forward-data" {
		t.Fatalf("data = %q, want forward-data", found.GetData())
	}
	if err := recvTestDexValue(t, fastErr, "fast responder result"); err != nil {
		t.Fatalf("fast responder: %v", err)
	}
	waitTestDexCondition(t, "slow pending request to clear after forwarded success", func() bool {
		return testDexPendingLen(slow) == 0
	})
}

type testDexResolverHandler struct {
	values []directive.Value
}

func (h *testDexResolverHandler) AddValue(value directive.Value) (uint32, bool) {
	h.values = append(h.values, value)
	return uint32(len(h.values)), true
}

func (h *testDexResolverHandler) RemoveValue(id uint32) (directive.Value, bool) {
	if id == 0 || int(id) > len(h.values) {
		return nil, false
	}
	value := h.values[id-1]
	h.values[id-1] = nil
	return value, true
}

func (h *testDexResolverHandler) CountValues(bool) int {
	return len(h.values)
}

func (h *testDexResolverHandler) ClearValues() []uint32 {
	ids := make([]uint32, len(h.values))
	for i := range h.values {
		ids[i] = uint32(i + 1)
	}
	h.values = nil
	return ids
}

func (h *testDexResolverHandler) MarkIdle(bool) {}

func (h *testDexResolverHandler) AddValueRemovedCallback(uint32, func()) func() {
	return func() {}
}

func (h *testDexResolverHandler) AddResolverRemovedCallback(func()) func() {
	return func() {}
}

func (h *testDexResolverHandler) AddResolver(directive.Resolver, func()) func() {
	return func() {}
}

func assertTestDexNotFoundValue(t *testing.T, handler *testDexResolverHandler) {
	// Verify the resolver publishes exactly one successful block miss.
	t.Helper()
	if len(handler.values) != 1 {
		t.Fatalf("values = %d, want 1", len(handler.values))
	}
	value, ok := handler.values[0].(dex.LookupBlockFromNetworkValue)
	if !ok {
		t.Fatalf("value type = %T, want LookupBlockFromNetworkValue", handler.values[0])
	}
	if len(value.GetData()) != 0 {
		t.Fatalf("data = %q, want empty", value.GetData())
	}
	if err := value.GetError(); err != nil {
		t.Fatal(err)
	}
}

// _ is a type assertion.
var _ directive.ResolverHandler = (*testDexResolverHandler)(nil)

type testDexMountedLink struct {
	remote peer.ID
}

func (l *testDexMountedLink) GetLinkUUID() uint64 {
	return 1
}

func (l *testDexMountedLink) GetTransportUUID() uint64 {
	return 1
}

func (l *testDexMountedLink) GetRemoteTransportUUID() uint64 {
	return 1
}

func (l *testDexMountedLink) GetLocalPeer() peer.ID {
	return peer.ID("local")
}

func (l *testDexMountedLink) GetRemotePeer() peer.ID {
	return l.remote
}

func (l *testDexMountedLink) OpenMountedStream(
	context.Context,
	protocol.ID,
	stream.OpenOpts,
) (link.MountedStream, error) {
	return nil, nil
}

// _ is a type assertion.
var _ link.MountedLink = (*testDexMountedLink)(nil)

type testDexMountedStream struct {
	stream net.Conn
	link   link.MountedLink
	remote peer.ID
}

func (s *testDexMountedStream) GetStream() stream.Stream {
	return s.stream
}

func (s *testDexMountedStream) GetProtocolID() protocol.ID {
	return DexProtocolID
}

func (s *testDexMountedStream) GetOpenOpts() stream.OpenOpts {
	return stream.OpenOpts{}
}

func (s *testDexMountedStream) GetPeerID() peer.ID {
	return s.remote
}

func (s *testDexMountedStream) GetLink() link.MountedLink {
	return s.link
}

// _ is a type assertion.
var _ link.MountedStream = (*testDexMountedStream)(nil)

func newTestDexMountedStreamPair(remote peer.ID) (*testDexMountedStream, *stream_packet.Session, func()) {
	// Construct connected packet endpoints with shared test cleanup.
	localConn, remoteConn := net.Pipe()
	ms := &testDexMountedStream{
		stream: localConn,
		link:   &testDexMountedLink{remote: remote},
		remote: remote,
	}
	remoteSess := stream_packet.NewSession(remoteConn, maxMessageSize)
	cleanup := func() {
		_ = localConn.Close()
		_ = remoteSess.Close()
	}
	return ms, remoteSess, cleanup
}

func newTestPeerSessionPair(c *Controller, remote peer.ID) (*peerSession, *stream_packet.Session, func()) {
	ms, remoteSess, cleanup := newTestDexMountedStreamPair(remote)
	sess := newPeerSession(c, c.le.WithField("remote-peer", remote.String()), ms, nil)
	return sess, remoteSess, func() {
		sess.close()
		cleanup()
	}
}

func newTestDexSolicitController() *Controller {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return &Controller{
		le:       logrus.NewEntry(logger),
		cc:       &Config{MaxForwardHops: 1},
		sessions: make(map[string]*peerSession),
		settled:  true,
	}
}

func testDexBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(
		[]byte(data),
		&block.PutOpts{HashType: hash.HashType_HashType_SHA256},
	)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func getTestDexSession(c *Controller, remote peer.ID) *peerSession {
	var sess *peerSession
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sess = c.sessions[remote.String()]
	})
	return sess
}

func waitTestDexSession(t *testing.T, c *Controller, remote peer.ID) *peerSession {
	t.Helper()
	var sess *peerSession
	waitTestDexCondition(t, "session "+remote.String(), func() bool {
		sess = getTestDexSession(c, remote)
		return sess != nil
	})
	return sess
}

func testDexPendingLen(s *peerSession) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return len(s.pending)
}

func waitTestDexCondition(t *testing.T, name string, fn func() bool) {
	// Wait for the named session condition within the test deadline.
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if fn() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", name)
		case <-ticker.C:
		}
	}
}

func recvTestDexValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()
	select {
	case val := <-ch:
		return val
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
	var zero T
	return zero
}

func assertNoTestDexValue[T any](t *testing.T, ch <-chan T, name string) {
	t.Helper()
	select {
	case val := <-ch:
		t.Fatalf("unexpected %s: %#v", name, val)
	case <-time.After(100 * time.Millisecond):
	}
}
