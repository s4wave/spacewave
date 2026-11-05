package dex_solicit

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
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

	// sendMtx guards sendQueue and sending. It is never held during a write.
	sendMtx sync.Mutex
	// sendQueue holds the messages waiting for the writer, oldest first.
	sendQueue []*queuedSend
	// sending indicates a writer goroutine is draining sendQueue.
	sending bool

	// mtx guards pending and handling. It is never held during a write, so
	// the read loop dispatches responses while a large block is sent.
	mtx sync.Mutex
	// pending maps local request ids to the channels awaiting responses.
	pending map[uint32]chan *DexMessage
	// handling maps the remote peer's request ids to the requests this side
	// is serving.
	handling map[uint32]*handledRequest
}

// queuedSend states: a queued message is either written or abandoned by its
// sender, whichever claims it first.
const (
	sendQueued int32 = iota
	sendWriting
	sendAbandoned
)

// queuedSend is a message waiting for the session writer.
type queuedSend struct {
	ctx, requestCtx context.Context
	msg             *DexMessage
	access          RequestAccess
	closeOnCancel   bool
	// state is sendQueued until the writer or the sender claims it.
	state atomic.Int32
	// done receives the write result.
	done chan error
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

// handleRequest retains read authority through lookup, forwarding and response.
// A request canceled by the peer sends no response.
func (s *peerSession) handleRequest(
	ctx context.Context,
	h *handledRequest,
	req *DexMessage,
) {
	// Release the request registration on every authorization and lookup outcome.
	defer s.finishRequest(req.GetRequestId(), h)
	var access RequestAccess
	if s.c.authorizeRequest != nil {
		var err error
		access, err = s.c.authorizeRequest(ctx, s.c.cc, s.ms.GetPeerID())
		if err != nil {
			s.sendRequestError(ctx, req.GetRequestId(), err)
			return
		}
		defer access.Release()
		if err := access.Check(); err != nil {
			s.sendRequestError(ctx, req.GetRequestId(), err)
			return
		}
	}

	// Cancel blocked reads and forwarding when the authority source revokes access.
	requestCtx, cancel := context.WithCancelCause(ctx)
	watcher := routine.NewRoutineContainer()
	if access != nil {
		watcher.SetRoutine(func(ctx context.Context) error {
			cancel(access.WaitRevoked(ctx))
			return nil
		})
		watcher.SetContext(requestCtx, false)
	}
	defer func() {
		cancel(nil)
		if exited, _ := watcher.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Recheck authority after taking the write slot, before disclosing any block.
	resp := s.buildResponse(requestCtx, req)
	if err := s.sendMessage(ctx, requestCtx, resp, access, false); err != nil && ctx.Err() == nil {
		s.le.WithError(err).Debug("dex session send error")
	}
}

// sendRequestError reports a rejection without disclosing block data or refs.
func (s *peerSession) sendRequestError(ctx context.Context, id uint32, err error) {
	if sendErr := s.sendMessage(ctx, ctx, &DexMessage{RequestId: id, IsResponse: true, Error: err.Error()}, nil, true); sendErr != nil && ctx.Err() == nil {
		s.le.WithError(sendErr).Debug("dex session send error")
	}
}

// buildResponse reads locally, then forwards within the configured hop budget.
func (s *peerSession) buildResponse(ctx context.Context, req *DexMessage) *DexMessage {
	// Prepare the response without disclosing data until lookup succeeds.
	ref := req.GetRef()
	resp := &DexMessage{RequestId: req.GetRequestId(), IsResponse: true}

	// Reject a block request that cannot identify stored data.
	if ref == nil || ref.GetEmpty() {
		resp.Error = "empty block ref"
		return resp
	}

	// Check the local bucket before forwarding.
	local, err := s.lookupLocalBlock(ctx, ref)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	if local != nil {
		resp.SetBlock(local)
		return resp
	}

	// Forward to other peers if hops remain.
	// Clamp to configured max so a malicious peer cannot amplify traffic.
	maxHops := s.c.cc.GetMaxForwardHops()
	hops := min(req.GetRemainingHops(), maxHops)
	if hops > 0 {
		found, err := s.c.forwardToPeers(ctx, ref, hops-1, s)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		if found != nil {
			resp.SetBlock(found)
		}
	}
	return resp
}

// sendMessage queues a message for the session writer and waits for its write.
// A sender whose ctx ends before the write starts drops its message; a write
// in progress completes on the stream it already holds.
func (s *peerSession) sendMessage(ctx, requestCtx context.Context, resp *DexMessage, access RequestAccess, closeOnCancel bool) error {
	// Refuse a write that ctx already canceled.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Queue the message, starting a writer when none is draining the queue.
	q := &queuedSend{
		ctx:           ctx,
		requestCtx:    requestCtx,
		msg:           resp,
		access:        access,
		closeOnCancel: closeOnCancel,
		done:          make(chan error, 1),
	}
	s.sendMtx.Lock()
	s.sendQueue = append(s.sendQueue, q)
	start := !s.sending
	s.sending = true
	s.sendMtx.Unlock()
	if start {
		go s.writeQueued()
	}

	// Wait for the write, abandoning the message if ctx ends while it waits.
	select {
	case err := <-q.done:
		return err
	case <-ctx.Done():
		if q.state.CompareAndSwap(sendQueued, sendAbandoned) {
			return ctx.Err()
		}
		return <-q.done
	}
}

// maxSendBatch bounds the payload bytes of one batched write. A larger message
// still goes out alone.
const maxSendBatch = 1 << 20

// writeQueued writes queued messages in arrival order until the queue is
// empty. One running writer drains every message queued behind it and writes
// them in batches, so a busy session pays one transport wait per batch rather
// than one sender wakeup and one transport wait per message.
func (s *peerSession) writeQueued() {
	for {
		// Take the queued messages, or stop when nothing is queued.
		s.sendMtx.Lock()
		queue := s.sendQueue
		s.sendQueue = nil
		if len(queue) == 0 {
			s.sending = false
			s.sendMtx.Unlock()
			return
		}
		s.sendMtx.Unlock()

		// Write them in bounded batches.
		for len(queue) != 0 {
			queue = queue[s.writeBatch(queue):]
		}
	}
}

// preparedSend is a claimed message checked at the write slot.
type preparedSend struct {
	q             *queuedSend
	msg           *DexMessage
	closeOnCancel bool
}

// writeBatch claims messages from the front of queue up to maxSendBatch bytes
// and writes them together. It returns the number of entries it consumed.
// A revoked in-flight payload closes its session because a partial frame
// cannot be reused.
func (s *peerSession) writeBatch(queue []*queuedSend) int {
	// Claim the messages whose senders still wait, checking each one's
	// authority at the write slot. A message that would carry the batch past
	// maxSendBatch waits for the next batch, so small frames do not wait
	// behind a large one.
	var batch []preparedSend
	n, size := 0, 0
	for ; n < len(queue); n++ {
		q := queue[n]
		if len(batch) != 0 && size+len(q.msg.GetData()) > maxSendBatch {
			break
		}
		if !q.state.CompareAndSwap(sendQueued, sendWriting) {
			continue
		}
		p, err := prepareSend(q)
		if err != nil {
			q.done <- err
			continue
		}
		batch = append(batch, p)
		size += len(p.msg.GetData())
	}
	if len(batch) == 0 {
		return n
	}

	// Watch each message's cancellation and authority while the batch writes.
	msgs := make([]protobuf_go_lite.Message, len(batch))
	var stops []func() bool
	for i, p := range batch {
		msgs[i] = p.msg
		stops = s.watchWrite(stops, p)
	}

	// Write the batch, then report the result to every sender.
	err := s.sess.SendMsgs(msgs...)
	for _, stop := range stops {
		stop()
	}
	peerID := s.ms.GetPeerID().String()
	for _, p := range batch {
		if err == nil && p.msg.GetIsResponse() {
			s.c.recordTransfer(peerID, len(p.msg.GetData()), 0)
		}
		p.q.done <- err
	}
	return n
}

// prepareSend checks a claimed message's sender and authority at the write
// slot, replacing a revoked response entirely so neither data nor child refs
// escape.
func prepareSend(q *queuedSend) (preparedSend, error) {
	// Skip a message whose sender ended before the write started.
	if err := q.ctx.Err(); err != nil {
		return preparedSend{}, err
	}

	// Deny a response whose requester lost authority or ended.
	p := preparedSend{q: q, msg: q.msg, closeOnCancel: q.closeOnCancel}
	if q.access != nil {
		if err := q.access.Check(); err != nil {
			p.msg = &DexMessage{RequestId: q.msg.GetRequestId(), IsResponse: true, Error: err.Error()}
			p.closeOnCancel = true
		} else if err = context.Cause(q.requestCtx); err != nil {
			p.msg = &DexMessage{RequestId: q.msg.GetRequestId(), IsResponse: true, Error: err.Error()}
		}
	}
	return p, nil
}

// watchWrite appends the stops of the callbacks that close the stream while p
// writes: when a canceled sender's denial blocks, and when a payload's
// requester loses authority.
func (s *peerSession) watchWrite(stops []func() bool, p preparedSend) []func() bool {
	// Close the stream if a canceled request blocks while writing its denial.
	if p.closeOnCancel {
		stops = append(stops, context.AfterFunc(p.q.ctx, s.close))
	}

	// Interrupt an in-progress payload write if its requester loses authority.
	if access := p.q.access; p.msg.GetFound() && access != nil {
		stops = append(stops, context.AfterFunc(p.q.requestCtx, func() {
			if access.Check() != nil {
				s.close()
			}
		}))
	}
	return stops
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
		return nil, errSessionClosed
	}

	// Register the pending request before sending it.
	id := s.nextID.Add(1)
	ch := make(chan *DexMessage, 1)
	s.mtx.Lock()
	if s.closed.Load() {
		s.mtx.Unlock()
		return nil, errSessionClosed
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
			return nil, errSessionClosed
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
	return s.sendMessage(ctx, ctx, msg, nil, false)
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
