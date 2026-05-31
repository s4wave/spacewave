package dex_solicit

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestDexSolicitController()
	remote := peer.ID("peer-a")
	ref := testDexBlockRef(t, "replace")

	firstMS, firstRemote, firstCleanup := newTestDexMountedStreamPair(remote)
	defer firstCleanup()
	c.handleSolicitedStream(ctx, link_solicit.NewSolicitMountedStream(firstMS))
	first := waitTestDexSession(t, c, remote)

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

	requestDone := make(chan error, 1)
	go func() {
		_, _, err := first.requestBlock(ctx, ref, 0)
		requestDone <- err
	}()

	req := recvTestDexValue(t, remoteReq, "old session request")
	if req.GetRequestId() == 0 {
		t.Fatal("request id was not assigned")
	}

	secondMS, secondRemote, secondCleanup := newTestDexMountedStreamPair(remote)
	defer secondCleanup()
	c.handleSolicitedStream(ctx, link_solicit.NewSolicitMountedStream(secondMS))
	second := waitTestDexSession(t, c, remote)
	if second == first {
		t.Fatal("replacement kept old session")
	}

	err := recvTestDexValue(t, requestDone, "old pending request result")
	if err == nil || !strings.Contains(err.Error(), "session closed") {
		t.Fatalf("old request err = %v, want session closed", err)
	}
	select {
	case err := <-remoteErr:
		t.Fatalf("old remote read failed: %v", err)
	default:
	}

	if err := first.waitExited(ctx); err != nil {
		t.Fatal("old session exit:", err)
	}
	if current := getTestDexSession(c, remote); current != second {
		t.Fatalf("stale cleanup removed replacement session: got %p, want %p", current, second)
	}
	_ = secondRemote.Close()
}

func TestPeerSessionCloseWakesPendingRequestsWithoutRunLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "close")
	sess, remote, cleanup := newTestPeerSessionPair(c, peer.ID("close-peer"))
	defer cleanup()

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

	requestDone := make(chan error, 1)
	go func() {
		_, _, err := sess.requestBlock(ctx, ref, 0)
		requestDone <- err
	}()

	req := recvTestDexValue(t, remoteReq, "request before close")
	if req.GetRequestId() == 0 {
		t.Fatal("request id was not assigned")
	}

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

func TestLookupResolverQueryPeersCancelsLosersAfterFirstSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "query")
	fast, fastRemote, fastCleanup := newTestPeerSessionPair(c, peer.ID("fast"))
	defer fastCleanup()
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	fast.start(ctx)
	slow.start(ctx)

	slowReceived := make(chan struct{})
	go func() {
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err == nil {
			close(slowReceived)
		}
	}()

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

	resolver := &lookupResolver{c: c, ref: ref}
	data, found := resolver.queryPeers(ctx, []*peerSession{slow, fast})
	if !found {
		t.Fatal("queryPeers did not return first successful response")
	}
	if string(data) != "fast-data" {
		t.Fatalf("data = %q, want fast-data", data)
	}
	if err := recvTestDexValue(t, fastErr, "fast responder result"); err != nil {
		t.Fatalf("fast responder: %v", err)
	}
	waitTestDexCondition(t, "slow pending request to clear after first success", func() bool {
		return testDexPendingLen(slow) == 0
	})
}

func TestLookupResolverQueryPeersDeadlineClearsPendingRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "deadline")
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	slow.start(ctx)

	received := make(chan struct{})
	go func() {
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err == nil {
			close(received)
		}
	}()

	resolver := &lookupResolver{c: c, ref: ref}
	data, found := resolver.queryPeers(ctx, []*peerSession{slow})
	if found {
		t.Fatalf("queryPeers returned data after caller deadline: %q", data)
	}
	recvTestDexValue(t, received, "deadline request")
	waitTestDexCondition(t, "slow pending request to clear after caller deadline", func() bool {
		return testDexPendingLen(slow) == 0
	})
}

func TestControllerForwardToPeersExcludesOrigin(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "forward")
	origin, originRemote, originCleanup := newTestPeerSessionPair(c, peer.ID("origin"))
	defer originCleanup()
	origin.start(ctx)

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions["origin"] = origin
		broadcast()
	})

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

	data, found := c.forwardToPeers(ctx, ref, 0, origin)
	if found {
		t.Fatalf("forwardToPeers used excluded origin session and returned %q", data)
	}
	assertNoTestDexValue(t, originErr, "origin request")
}

func TestControllerForwardToPeersCancelsLosersAfterFirstSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestDexSolicitController()
	ref := testDexBlockRef(t, "forward")
	fast, fastRemote, fastCleanup := newTestPeerSessionPair(c, peer.ID("fast"))
	defer fastCleanup()
	slow, slowRemote, slowCleanup := newTestPeerSessionPair(c, peer.ID("slow"))
	defer slowCleanup()
	fast.start(ctx)
	slow.start(ctx)

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions["fast"] = fast
		c.sessions["slow"] = slow
		broadcast()
	})

	slowReceived := make(chan struct{})
	go func() {
		var req DexMessage
		if err := slowRemote.RecvMsg(&req); err == nil {
			close(slowReceived)
		}
	}()

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

	data, found := c.forwardToPeers(ctx, ref, 0, nil)
	if !found {
		t.Fatal("forwardToPeers did not return first successful response")
	}
	if string(data) != "forward-data" {
		t.Fatalf("data = %q, want forward-data", data)
	}
	if err := recvTestDexValue(t, fastErr, "fast responder result"); err != nil {
		t.Fatalf("fast responder: %v", err)
	}
	waitTestDexCondition(t, "slow pending request to clear after forwarded success", func() bool {
		return testDexPendingLen(slow) == 0
	})
}

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
