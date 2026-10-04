package dex_solicit

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/link"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// peerSession manages a bidirectional DEX session with a remote peer.
// Both sides can send requests and receive responses concurrently.
type peerSession struct {
	c  *Controller
	le *logrus.Entry
	ms link.MountedStream

	sess       *stream_packet.Session
	runRoutine *routine.RoutineContainer
	onExit     func()
	nextID     atomic.Uint32
	closed     atomic.Bool

	// sendMtx serializes SendMsg writes in arrival order. A sender waits for
	// it with its own context, so a canceled request does not wait behind a
	// long block write.
	sendMtx csync.Mutex

	// mtx guards pending and handling. It is never held during a write, so
	// the read loop dispatches responses while a large block is sent.
	mtx sync.Mutex
	// pending maps local request ids to the channels awaiting responses.
	pending map[uint32]chan *DexMessage
	// handling maps the remote peer's request ids to the requests this side
	// is serving.
	handling map[uint32]*handledRequest
}

// handledRequest is an incoming request this side is serving.
type handledRequest struct {
	// cancel stops serving the request.
	cancel context.CancelFunc
}

func newPeerSession(
	c *Controller,
	le *logrus.Entry,
	ms link.MountedStream,
	onExit func(),
) *peerSession {
	s := &peerSession{
		c:        c,
		le:       le,
		ms:       ms,
		sess:     stream_packet.NewSession(ms.GetStream(), maxMessageSize),
		onExit:   onExit,
		pending:  make(map[uint32]chan *DexMessage),
		handling: make(map[uint32]*handledRequest),
	}
	s.runRoutine = routine.NewRoutineContainer()
	_, _ = s.runRoutine.SetRoutine(func(ctx context.Context) error {
		s.run(ctx)
		if s.onExit != nil {
			s.onExit()
		}
		return nil
	})
	return s
}

func (s *peerSession) start(ctx context.Context) {
	s.runRoutine.SetContext(ctx, false)
}

func (s *peerSession) waitExited(ctx context.Context) error {
	return s.runRoutine.WaitExited(ctx, true, nil)
}

// run starts the session, reading messages in a loop and dispatching
// responses to pending requests, cancels to handled requests, and incoming
// requests to new handlers.
func (s *peerSession) run(ctx context.Context) {
	// Mark the session closed, wake pending requests, and stop handlers on
	// exit.
	defer func() {
		s.closed.Store(true)
		s.sess.Close()
		s.wakePending()
		s.cancelHandling()
	}()

	// Read and dispatch session messages.
	for {
		var msg DexMessage
		if err := s.sess.RecvMsg(&msg); err != nil {
			if err != io.EOF && ctx.Err() == nil {
				s.le.WithError(err).Debug("dex session read error")
			}
			return
		}

		switch {
		case msg.GetCancel():
			s.cancelRequest(msg.GetRequestId())
		case msg.GetIsResponse():
			s.dispatchResponse(&msg)
		default:
			s.startRequest(ctx, &msg)
		}
	}
}

// dispatchResponse delivers a response to the pending request with its id.
// A response for a request that already finished is dropped.
func (s *peerSession) dispatchResponse(msg *DexMessage) {
	// Count the payload and claim the waiting request.
	s.c.recordTransfer(s.ms.GetPeerID().String(), 0, len(msg.GetData()))
	s.mtx.Lock()
	ch, ok := s.pending[msg.GetRequestId()]
	if ok {
		delete(s.pending, msg.GetRequestId())
	}
	s.mtx.Unlock()
	if ok {
		ch <- msg
	}
}

// startRequest registers an incoming request so a cancel can stop it, then
// handles it in its own goroutine.
func (s *peerSession) startRequest(ctx context.Context, req *DexMessage) {
	// Replace any request the peer reissued under the same id.
	reqCtx, cancel := context.WithCancel(ctx)
	h := &handledRequest{cancel: cancel}
	id := req.GetRequestId()
	s.mtx.Lock()
	if prev := s.handling[id]; prev != nil {
		prev.cancel()
	}
	s.handling[id] = h
	s.mtx.Unlock()
	go s.handleRequest(reqCtx, h, req)
}

// cancelRequest stops the handled request with id. It does nothing when the
// request already finished.
func (s *peerSession) cancelRequest(id uint32) {
	// Remove the request and stop its handler outside the lock.
	s.mtx.Lock()
	h := s.handling[id]
	delete(s.handling, id)
	s.mtx.Unlock()
	if h != nil {
		h.cancel()
	}
}

// cancelHandling stops every handled request.
func (s *peerSession) cancelHandling() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for id, h := range s.handling {
		h.cancel()
		delete(s.handling, id)
	}
}

// handleRequest handles an incoming block request and sends a response. A
// request canceled by the peer sends no response.
func (s *peerSession) handleRequest(
	ctx context.Context,
	h *handledRequest,
	req *DexMessage,
) {
	// Prepare a response that is sent when request handling finishes, and
	// deregister the request afterward.
	ref := req.GetRef()
	resp := &DexMessage{
		RequestId:  req.GetRequestId(),
		IsResponse: true,
	}
	defer func() {
		err := s.sendMsg(ctx, resp)
		if err != nil && ctx.Err() == nil {
			s.le.WithError(err).Debug("dex session send error")
		}
		s.finishRequest(req.GetRequestId(), h)
	}()

	// Reject a block request that cannot identify stored data.
	if ref == nil || ref.GetEmpty() {
		resp.Error = "empty block ref"
		return
	}

	// Check the local bucket before forwarding.
	local, err := s.lookupLocalBlock(ctx, ref)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	if local != nil {
		resp.SetBlock(local)
		return
	}

	// Forward to other peers if hops remain.
	// Clamp to configured max so a malicious peer cannot amplify traffic.
	maxHops := s.c.cc.GetMaxForwardHops()
	hops := min(req.GetRemainingHops(), maxHops)
	if hops > 0 {
		found, err := s.c.forwardToPeers(ctx, ref, hops-1, s)
		if err != nil {
			resp.Error = err.Error()
			return
		}
		if found != nil {
			resp.SetBlock(found)
		}
	}
}

// finishRequest removes a finished request from the handling map unless a
// newer request reused its id, and releases its context.
func (s *peerSession) finishRequest(id uint32, h *handledRequest) {
	s.mtx.Lock()
	if s.handling[id] == h {
		delete(s.handling, id)
	}
	s.mtx.Unlock()
	h.cancel()
}

// lookupLocalBlock looks up a block and its refs in the local bucket store
// only. Returns nil when the block is not found.
func (s *peerSession) lookupLocalBlock(ctx context.Context, ref *block.BlockRef) (*DexMessage, error) {
	// Acquire the configured bucket lookup for this session.
	lkv, _, lkRel, err := bucket_lookup.ExBuildBucketLookup(ctx, s.c.b, false, s.c.cc.GetBucketId(), nil)
	if err != nil {
		return nil, err
	}
	defer lkRel.Release()

	// Open the bucket lookup used to read local blocks.
	lk, err := lkv.GetLookup(ctx)
	if err != nil || lk == nil {
		return nil, err
	}

	// Read the requested block without consulting remote peers.
	stored, err := lk.LookupStoredBlock(ctx, ref, bucket_lookup.WithLocalOnly())
	if err != nil || stored == nil {
		return nil, err
	}
	return &DexMessage{
		Found:     true,
		Data:      stored.Data,
		Refs:      stored.Refs,
		RefsKnown: stored.RefsKnown,
	}, nil
}

// requestBlock sends a block request and waits for the response. Returns the
// verified response, or nil when the peer does not have the block.
func (s *peerSession) requestBlock(ctx context.Context, ref *block.BlockRef, hops uint32) (*DexMessage, error) {
	// Reject block requests after the peer session closes.
	if s.closed.Load() {
		return nil, errors.New("session closed")
	}

	// Register the pending request before sending it.
	id := s.nextID.Add(1)
	ch := make(chan *DexMessage, 1)
	s.mtx.Lock()
	if s.closed.Load() {
		s.mtx.Unlock()
		return nil, errors.New("session closed")
	}
	s.pending[id] = ch
	s.mtx.Unlock()
	defer func() {
		s.mtx.Lock()
		delete(s.pending, id)
		s.mtx.Unlock()
	}()

	// Send the block request to the peer.
	req := &DexMessage{
		RequestId:     id,
		Ref:           ref,
		RemainingHops: hops,
	}
	if err := s.sendMsg(ctx, req); err != nil {
		return nil, err
	}

	// Await the response or request cancellation. A canceled request tells
	// the peer to stop serving it, so the peer does not spend its upload on
	// a response nobody reads. The cancel outlives ctx and ends with the
	// session, whose close fails the write.
	select {
	case <-ctx.Done():
		sendCtx := context.WithoutCancel(ctx)
		go func() {
			_ = s.sendMsg(sendCtx, &DexMessage{
				RequestId: id,
				Cancel:    true,
			})
		}()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return nil, errors.New("session closed")
		}
		if resp.GetError() != "" {
			return nil, errors.New(resp.GetError())
		}
		if !resp.GetFound() {
			return nil, nil
		}

		// Verify returned block data before reporting success.
		if err := ref.VerifyData(resp.GetData(), true); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// sendMsg sends a message with write serialization. It returns the context
// error without writing when ctx ends before the write starts.
func (s *peerSession) sendMsg(ctx context.Context, msg *DexMessage) error {
	// Wait for the write slot or the end of ctx.
	release, err := s.sendMtx.Lock(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		release()
		return err
	}

	// Write the message and account for response payloads.
	err = s.sess.SendMsg(msg)
	release()
	if err == nil && msg.GetIsResponse() {
		s.c.recordTransfer(s.ms.GetPeerID().String(), len(msg.GetData()), 0)
	}
	return err
}

// close closes the session stream.
func (s *peerSession) close() {
	if s.closed.CompareAndSwap(false, true) {
		if s.sess != nil {
			s.sess.Close()
		}
		s.wakePending()
	}
}

func (s *peerSession) wakePending() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for id, ch := range s.pending {
		select {
		case ch <- nil:
		default:
		}
		delete(s.pending, id)
	}
}
