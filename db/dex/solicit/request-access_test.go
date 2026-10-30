package dex_solicit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// TestRequestRevocationStopsBlockedPayload prevents a stalled requester from
// retaining access through a response write after its authority is revoked.
func TestRequestRevocationStopsBlockedPayload(t *testing.T) {
	// Start a production request handler with real bucket storage and pipe I/O.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	session, remote, access, ref := newAuthorizedRequest(t, ctx)
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)
	session.startRequest(ctx, &DexMessage{RequestId: 1, Ref: ref})

	// Revoke only after the unbuffered pipe blocks the response writer.
	recvTestDexValue(t, observed.writing, "blocked payload write")
	access.allowed.SetValue(false)
	recvTestDexValue(t, access.released, "revoked request release")
	if !session.closed.Load() {
		t.Fatal("revocation left the payload writer's stream open")
	}

	// No complete response or block data may survive the interrupted write.
	var response DexMessage
	if err := remote.RecvMsg(&response); err == nil {
		t.Fatalf("revoked response disclosed a frame: %v", &response)
	}
}

// TestRequestRechecksAuthorityAtWriteSlot prevents queued responses from using
// authority captured before another writer held the session.
func TestRequestRechecksAuthorityAtWriteSlot(t *testing.T) {
	// Prepare a real request over an unbuffered pipe with write observation.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	session, remote, access, ref := newAuthorizedRequest(t, ctx)
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)

	// Block the writer on a frame the remote has not read yet.
	go func() { _ = session.sendMsg(ctx, &DexMessage{RequestId: 99, Ref: ref}) }()
	recvTestDexValue(t, observed.writing, "blocked write slot")

	// Queue the request's response behind the blocked frame.
	session.startRequest(ctx, &DexMessage{RequestId: 2, Ref: ref})
	recvTestDexValue(t, access.watching, "request authority watch")

	// The queued response must read authority again after it gets the write slot.
	access.allowed.SetValue(false)
	var held, response DexMessage
	if err := remote.RecvMsg(&held); err != nil {
		t.Fatal(err)
	}
	if err := remote.RecvMsg(&response); err != nil {
		t.Fatal(err)
	}
	if response.GetFound() || len(response.GetData()) != 0 || len(response.GetRefs()) != 0 || !strings.Contains(response.GetError(), "access denied") {
		t.Fatalf("revoked queued response = %v", &response)
	}

	// An idle revoked requester must not tear down the reverse serving direction.
	recvTestDexValue(t, access.released, "queued request release")
	if session.closed.Load() {
		t.Fatal("denying a queued response closed the bidirectional stream")
	}
}

// TestRequestCancellationStopsBlockedDenial releases retained authority even
// when a rejected requester never reads its response.
func TestRequestCancellationStopsBlockedDenial(t *testing.T) {
	// Prepare a real request over an unread, unbuffered pipe.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	session, _, access, ref := newAuthorizedRequest(t, ctx)
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)

	// Reject the request before it reads any stored block.
	access.allowed.SetValue(false)
	session.startRequest(ctx, &DexMessage{RequestId: 3, Ref: ref})

	// Cancellation must release the blocked denial and its authority reference.
	recvTestDexValue(t, observed.writing, "blocked denial write")
	cancel()
	recvTestDexValue(t, access.released, "canceled denial release")
	if !session.closed.Load() {
		t.Fatal("request cancellation left the denial writer blocked")
	}
}

// TestRequestCancellationDuringPayloadKeepsAuthorizedStream verifies canceling
// one authorized request does not discard the peer's other requests.
func TestRequestCancellationDuringPayloadKeepsAuthorizedStream(t *testing.T) {
	// Prepare a production request on an unbuffered pipe with write observation.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	session, remote, access, ref := newAuthorizedRequest(t, ctx)
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)

	// Wait until an authorized block response is already being written.
	session.startRequest(ctx, &DexMessage{RequestId: 4, Ref: ref})
	recvTestDexValue(t, access.watching, "request authority watch")
	recvTestDexValue(t, observed.writing, "blocked authorized payload")

	// Cancel this request and let its existing write complete on the same session.
	session.cancelRequest(4)
	var response DexMessage
	if err := remote.RecvMsg(&response); err != nil {
		t.Fatalf("read the canceled request's in-flight response: %v", err)
	}
	if !response.GetFound() || !bytes.Equal(response.GetData(), []byte("authorized block")) {
		t.Fatalf("in-flight response = %v", &response)
	}
	recvTestDexValue(t, access.released, "canceled request release")
	if session.closed.Load() {
		t.Fatal("canceling one authorized request closed the bidirectional stream")
	}
}

// TestOutboundRequestCancellationKeepsSession verifies an outbound request's
// canceled write does not close a stream that can still complete the frame.
func TestOutboundRequestCancellationKeepsSession(t *testing.T) {
	// Prepare the stream and a request-specific cancellation context.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	// Build the real request fixture and isolate this write's cancellation.
	session, remote, _, ref := newAuthorizedRequest(t, ctx)
	writeCtx, cancelWrite := context.WithCancel(ctx)
	t.Cleanup(cancelWrite)

	// Replace the pipe with an observer that preserves its blocking behavior.
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)
	writeErr := make(chan error, 1)

	// Start the outbound request on the unbuffered stream.
	go func() { writeErr <- session.sendMsg(writeCtx, &DexMessage{RequestId: 5, Ref: ref}) }()

	// Cancel the caller while the request frame is blocked on the real pipe.
	recvTestDexValue(t, observed.writing, "blocked outbound request")
	cancelWrite()
	var request DexMessage
	if err := remote.RecvMsg(&request); err != nil {
		t.Fatalf("read the canceled request's in-flight frame: %v", err)
	}
	if request.GetRequestId() != 5 {
		t.Fatalf("outbound request ID = %d", request.GetRequestId())
	}
	if err := recvTestDexValue(t, writeErr, "outbound request write"); err != nil {
		t.Fatal(err)
	}
	if session.closed.Load() {
		t.Fatal("canceling one outbound request closed the bidirectional stream")
	}
}

// newAuthorizedRequest constructs a production DEX request path over a real pipe.
func newAuthorizedRequest(t *testing.T, ctx context.Context) (*peerSession, *stream_packet.Session, *testRequestAccess, *block.BlockRef) {
	// Configure a real bucket and retain the authority source for its request.
	t.Helper()
	node := newTestRelayNode(t, ctx, logrus.NewEntry(logrus.New()), "authorized", []byte("authority"), 0)
	access := newTestRequestAccess()
	node.dex.authorizeRequest = func(context.Context, *Config, peer.ID) (RequestAccess, error) {
		return access, nil
	}
	bucketAPI, _, bucketRef, err := bucket.ExBuildBucketAPI(ctx, node.tb.Bus, false, node.bucketID, node.tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bucketRef.Release)

	// Store a leaf and child refs that a rejected response must not disclose.
	child, _, err := bucketAPI.GetBucket().PutBlock(ctx, []byte("child"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := bucketAPI.GetBucket().PutBlock(ctx, []byte("authorized block"), &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}

	// Keep the real peer session's receive side unread until the test is ready.
	session, remote, cleanup := newTestPeerSessionPair(node.dex, peer.ID("requester"))
	t.Cleanup(cleanup)
	return session, remote, access, ref
}

// TestQueuedSendAbandonedOnCancel verifies a sender that gives up while its
// message waits for the writer drops that message without stalling the queue.
func TestQueuedSendAbandonedOnCancel(t *testing.T) {
	// Hold the write slot with a frame blocked on the unread pipe.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	session, remote, _, ref := newAuthorizedRequest(t, ctx)
	mounted := session.ms.(*testDexMountedStream)
	observed := &writeObserverConn{Conn: mounted.stream, writing: make(chan struct{}, 1)}
	mounted.stream = observed
	session.sess = stream_packet.NewSession(mounted.GetStream(), maxMessageSize)

	// Block the writer on a frame the remote has not read yet.
	go func() { _ = session.sendMsg(ctx, &DexMessage{RequestId: 1, Ref: ref}) }()
	recvTestDexValue(t, observed.writing, "blocked write slot")

	// Queue a message behind it and cancel its sender before the write starts.
	queuedCtx, cancelQueued := context.WithCancel(ctx)
	queuedErr := make(chan error, 1)
	go func() { queuedErr <- session.sendMsg(queuedCtx, &DexMessage{RequestId: 2, Ref: ref}) }()
	for {
		session.sendMtx.Lock()
		queued := len(session.sendQueue)
		session.sendMtx.Unlock()
		if queued != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancelQueued()
	if err := recvTestDexValue(t, queuedErr, "abandoned send"); err == nil {
		t.Fatal("abandoned send reported a write")
	}

	// A later message follows the held frame and the abandoned one is absent.
	laterErr := make(chan error, 1)
	go func() { laterErr <- session.sendMsg(ctx, &DexMessage{RequestId: 3, Ref: ref}) }()
	for _, want := range []uint32{1, 3} {
		var msg DexMessage
		if err := remote.RecvMsg(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.GetRequestId() != want {
			t.Fatalf("frame request ID = %d, want %d", msg.GetRequestId(), want)
		}
	}
	if err := recvTestDexValue(t, laterErr, "later send"); err != nil {
		t.Fatal(err)
	}
}

// TestWriteBatchLeavesLargeMessageForNextBatch verifies a message that would
// carry a batch past maxSendBatch waits for the next batch, so the small frame
// ahead of it does not wait for the large write.
func TestWriteBatchLeavesLargeMessageForNextBatch(t *testing.T) {
	// Drain the remote side so each batch write completes.
	session, remote, cleanup := newTestPeerSessionPair(newTestDexSolicitController(), "remote")
	t.Cleanup(cleanup)
	go func() {
		for {
			if err := remote.RecvMsg(&DexMessage{}); err != nil {
				return
			}
		}
	}()

	// Queue a small frame ahead of a full-size one.
	queue := []*queuedSend{
		{ctx: t.Context(), msg: &DexMessage{RequestId: 1, Data: make([]byte, 100)}, done: make(chan error, 1)},
		{ctx: t.Context(), msg: &DexMessage{RequestId: 2, Data: make([]byte, maxSendBatch)}, done: make(chan error, 1)},
	}

	// The first batch takes only the small frame, and the next takes the other.
	if n := session.writeBatch(queue); n != 1 {
		t.Fatalf("first batch consumed %d messages, want 1", n)
	}
	if n := session.writeBatch(queue[1:]); n != 1 {
		t.Fatalf("second batch consumed %d messages, want 1", n)
	}
	for _, q := range queue {
		if err := <-q.done; err != nil {
			t.Fatal(err)
		}
	}
}
