package dex_solicit

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

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
	// endErr is why the session ended, set once by the first close or read
	// failure.
	endErr atomic.Pointer[error]

	// sendMtx guards sendQueue and sending. It is never held during a write.
	sendMtx sync.Mutex
	// sendQueue holds the messages waiting for the writer, oldest first.
	sendQueue []*queuedSend
	// sending indicates a writer goroutine is draining sendQueue.
	sending bool

	// mtx guards pending, handling and the requests in each. It is never
	// held during a write, so the read loop dispatches responses while a
	// large block is sent.
	mtx sync.Mutex
	// pending maps local request ids to the requests awaiting responses.
	pending map[uint32]*pendingRequest
	// handling maps the remote peer's request ids to the requests this side
	// is serving.
	handling map[uint32]*handledRequest
	// rate measures the session's delivery while pending is not empty.
	rate deliveryRate
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

// countedStream counts each read's bytes in the session's received count,
// which the session's delivery rate and the progress of each read pending on
// it measure.
type countedStream struct {
	io.ReadWriteCloser
	// received is the session's count.
	received *atomic.Uint64
}

// Read reads from the stream and counts the bytes read.
func (s countedStream) Read(p []byte) (int, error) {
	n, err := s.ReadWriteCloser.Read(p)
	s.received.Add(uint64(n)) //nolint:gosec // Read never returns a negative count.
	return n, err
}

// errPeerClosedStream reports a session whose peer closed the stream.
var errPeerClosedStream = errors.New("the peer closed the stream")

// errSessionClosedLocally reports a session this side closed.
var errSessionClosedLocally = errors.New("closed by this side")

// errDenialWriteBlocked reports a session closed because the denial of a
// canceled request blocked its write.
var errDenialWriteBlocked = errors.New("closed the stream: the denial of a canceled request blocked its write")

// errAccessRevokedInWrite reports a session closed because its requester lost
// read access while a block payload was written.
var errAccessRevokedInWrite = errors.New("closed the stream: the requester lost read access during a block write")

// handledRequest is an incoming request this side is serving.
type handledRequest struct {
	// cancel stops serving the request.
	cancel context.CancelFunc
	// pruned holds the keys of the refs the requester pruned from the
	// request's subtree walk. The session's mtx guards it.
	pruned map[string]struct{}
}

// pendingRequest is a request this side sent, awaiting its responses. The
// read loop queues each response and never waits for the requester.
type pendingRequest struct {
	// wake is signaled, without blocking, when a response is queued or the
	// request ends.
	wake chan struct{}

	// The fields below are guarded by the session's mtx.

	// queue holds the responses the requester has not taken, oldest first.
	queue []*DexMessage
	// bytes is the block data the request was answered with so far.
	bytes int
	// limit bounds bytes: a peer that sends more than the request asked for
	// ends it with errAnswerOverrun.
	limit int
	// end is why no response follows those queued: errSessionClosed or
	// errAnswerOverrun. It stays nil after the last response, which the
	// queue holds.
	end error
}

// signal wakes the requester waiting on p.
func (p *pendingRequest) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// errAnswerOverrun reports a peer that answered a request with more block data
// than it asked for.
var errAnswerOverrun = errors.New("the peer answered with more data than asked for")

func newPeerSession(
	c *Controller,
	le *logrus.Entry,
	ms link.MountedStream,
	onExit func(),
) *peerSession {
	// Build the session around its stream, counting what the stream reads.
	s := &peerSession{
		c:        c,
		le:       le,
		ms:       ms,
		onExit:   onExit,
		pending:  make(map[uint32]*pendingRequest),
		handling: make(map[uint32]*handledRequest),
	}
	s.sess = stream_packet.NewSession(countedStream{ReadWriteCloser: ms.GetStream(), received: &s.rate.received}, maxMessageSize)
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
			switch {
			case ctx.Err() != nil:
				err = ctx.Err()
			case err == io.EOF:
				err = errPeerClosedStream
			default:
				err = errors.Wrap(err, "read")
			}
			s.setEndErr(err)
			return
		}

		switch {
		case msg.GetCancel():
			s.cancelRequest(msg.GetRequestId())
		case msg.GetPrune():
			s.pruneRequest(msg.GetRequestId(), msg.GetRefs())
		case msg.GetIsResponse():
			s.dispatchResponse(&msg)
		default:
			s.startRequest(ctx, &msg)
		}
	}
}

// dispatchResponse queues a response for the pending request with its id,
// which a response without more finishes. A response for a request that
// already finished is dropped.
func (s *peerSession) dispatchResponse(msg *DexMessage) {
	// Count the payload and queue it for the waiting request.
	s.c.recordTransfer(s.ms.GetPeerID().String(), 0, len(msg.GetData()))
	id := msg.GetRequestId()
	s.mtx.Lock()
	p := s.pending[id]
	if p != nil {
		p.bytes += len(msg.GetData())
		switch {
		case p.bytes > p.limit:
			p.end = errAnswerOverrun
			delete(s.pending, id)
		case !msg.GetMore():
			p.queue = append(p.queue, msg)
			delete(s.pending, id)
		default:
			p.queue = append(p.queue, msg)
		}
	}
	s.rate.sample(time.Now(), len(s.pending) != 0)
	s.mtx.Unlock()
	if p != nil {
		p.signal()
	}
}

// take returns the responses queued for p, waiting for at least one. With
// none queued it returns why p ended, or ctx's error. The caller stops taking
// after a response without more.
func (s *peerSession) take(ctx context.Context, p *pendingRequest) ([]*DexMessage, error) {
	for {
		s.mtx.Lock()
		msgs, end := p.queue, p.end
		p.queue = nil
		s.mtx.Unlock()
		if len(msgs) != 0 {
			return msgs, nil
		}
		if end != nil {
			return nil, end
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.wake:
		}
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

// pruneRequest stops the subtree walk of the handled request with id from
// walking refs. It does nothing when the request already finished.
func (s *peerSession) pruneRequest(id uint32, refs []*block.BlockRef) {
	// Key the refs outside the lock.
	keys := make([]string, len(refs))
	for i, ref := range refs {
		keys[i] = ref.MapKey()
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	h := s.handling[id]
	if h == nil {
		return
	}
	if h.pruned == nil {
		h.pruned = make(map[string]struct{}, len(keys))
	}
	for _, key := range keys {
		h.pruned[key] = struct{}{}
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
			s.sendRequestDenied(ctx, req.GetRequestId(), err)
			return
		}
		defer access.Release()
		if err := access.Check(); err != nil {
			s.sendRequestDenied(ctx, req.GetRequestId(), err)
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

	// Answer the ref, with more set when the blocks under it follow: a subtree
	// request whose ref this side holds with its refs known, sent as data. A
	// subtree request reads as a walk, so a store laid out in graph order
	// reads the subtree's structure with the ref.
	if req.GetSubtreeBytes() != 0 {
		requestCtx = block.WithSubtreeRead(requestCtx)
	}
	resp, local := s.buildResponse(requestCtx, req)
	resp.More = local && req.GetSubtreeBytes() != 0 && !resp.GetLinked() &&
		resp.GetRefsKnown() && len(resp.GetRefs()) != 0

	// Recheck authority after taking the write slot, before disclosing any block.
	if err := s.sendMessage(ctx, requestCtx, resp, access, false); err != nil {
		if ctx.Err() == nil {
			s.le.WithError(err).Debug("dex session send error")
		}
		return
	}
	if resp.GetMore() {
		budget := min(req.GetSubtreeBytes(), maxSubtreeBytes)
		s.sendSubtree(ctx, requestCtx, h, req.GetRequestId(), int(budget), resp.GetRefs(), access)
	}
}

// maxSubtreeBytes bounds the block data a subtree request is answered with
// beyond its ref's block, whatever budget it asks for.
const maxSubtreeBytes = 4 << 20

// subtreeReadWidth is how many sibling refs a subtree walk reads at once.
const subtreeReadWidth = 64

// subtreeFrame is one level of a subtree walk: the sibling refs a sent block
// named that the walk has yet to read, and the blocks it read and has yet to
// send, in order.
type subtreeFrame struct {
	// refs are the siblings not yet read.
	refs []*block.BlockRef
	// held are the siblings read and not yet sent, with their refs.
	held []*block.StoredBlock
	// heldRefs are the refs of held.
	heldRefs []*block.BlockRef
}

// sendSubtree answers a subtree request after its first response. It walks
// the blocks under refs depth first, in pre-order, the order a graph-ordered
// pack holds them, reading up to subtreeReadWidth siblings at once from this
// side alone, and sends each block it holds with its refs known, as data, in a
// response carrying its ref and more. It stops when the next block would carry
// the data past budget bytes, the walk ends, or the request does; a block it
// lacks, holds without refs or holds linked ends the walk below it. Walking
// depth first, a stop leaves whole subtrees unsent, at most the unsent
// siblings along one path, rather than every block of a level. It walks no ref
// the requester pruned and none twice. A last response without more ends the
// stream, unless the request already ended.
func (s *peerSession) sendSubtree(ctx, requestCtx context.Context, h *handledRequest, id uint32, budget int, refs []*block.BlockRef, access RequestAccess) {
	// Read this side's blocks through one reader for the whole walk.
	r := &localReader{s: s}
	defer r.release()
	keys := make([]string, 0, subtreeReadWidth)
	chunk := make([]*block.BlockRef, 0, subtreeReadWidth)
	stored := make([]*block.StoredBlock, subtreeReadWidth)

	// Walk from the ref's children, sending each block at most once and
	// within budget.
	seen := make(map[string]struct{})
	stack := []*subtreeFrame{{refs: refs}}
	var sends []*queuedSend
	sent := 0
	for len(stack) != 0 && requestCtx.Err() == nil {
		top := stack[len(stack)-1]

		// Send the next block read at this level, then walk under it.
		if len(top.held) != 0 {
			blk, ref := top.held[0], top.heldRefs[0]
			top.held, top.heldRefs = top.held[1:], top.heldRefs[1:]
			s.mtx.Lock()
			_, pruned := h.pruned[ref.MapKey()]
			s.mtx.Unlock()
			if pruned {
				continue
			}
			if sent+len(blk.Data) > budget {
				break
			}
			sent += len(blk.Data)
			msg := storedBlockMessage(blk)
			msg.RequestId, msg.IsResponse, msg.Ref, msg.More = id, true, ref, true
			sends = append(sends, s.queueSend(ctx, requestCtx, msg, access, false))
			if len(blk.Refs) != 0 {
				stack = append(stack, &subtreeFrame{refs: blk.Refs})
			}
			continue
		}

		// Leave a level once every sibling is sent.
		if len(top.refs) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}

		// Wait for the writes, so the read sees the prunes they drew.
		for _, q := range sends {
			if err := waitSend(q); err != nil {
				return
			}
		}
		sends = sends[:0]

		// Take the level's next siblings the walk has not met, drop those the
		// requester pruned, and read them.
		chunk, keys = chunk[:0], keys[:0]
		for len(top.refs) != 0 && len(chunk) < subtreeReadWidth {
			ref := top.refs[0]
			top.refs = top.refs[1:]
			key := ref.MapKey()
			if _, ok := seen[key]; ok || key == "" {
				continue
			}
			seen[key] = struct{}{}
			chunk, keys = append(chunk, ref), append(keys, key)
		}
		s.mtx.Lock()
		kept := 0
		for i, ref := range chunk {
			if _, ok := h.pruned[keys[i]]; !ok {
				chunk[kept] = ref
				kept++
			}
		}
		s.mtx.Unlock()
		chunk = chunk[:kept]
		clear(stored)
		if err := r.read(requestCtx, chunk, stored); err != nil {
			s.le.WithError(err).Debug("dex subtree walk read failed")
			break
		}
		for i, blk := range stored[:len(chunk)] {
			if blk != nil && blk.RefsKnown && !blk.Linked {
				top.held, top.heldRefs = append(top.held, blk), append(top.heldRefs, chunk[i])
			}
		}
	}

	// Wait for the last writes, then end the stream.
	for _, q := range sends {
		if err := waitSend(q); err != nil {
			return
		}
	}
	if requestCtx.Err() == nil {
		end := &DexMessage{RequestId: id, IsResponse: true}
		if err := s.sendMessage(ctx, requestCtx, end, access, false); err != nil && ctx.Err() == nil {
			s.le.WithError(err).Debug("dex session send error")
		}
	}
}

// sendRequestDenied answers a request whose requester lacks read access,
// without disclosing block data or refs.
func (s *peerSession) sendRequestDenied(ctx context.Context, id uint32, err error) {
	if sendErr := s.sendMessage(ctx, ctx, deniedResponse(id, err), nil, true); sendErr != nil && ctx.Err() == nil {
		s.le.WithError(sendErr).Debug("dex session send error")
	}
}

// deniedResponse is the answer to request id when the requester lacks read
// access for reason err.
func deniedResponse(id uint32, err error) *DexMessage {
	return &DexMessage{RequestId: id, IsResponse: true, Denied: true, Error: err.Error()}
}

// buildResponse reads locally, then forwards within the configured hop budget.
// local reports a block found on this side.
func (s *peerSession) buildResponse(ctx context.Context, req *DexMessage) (resp *DexMessage, local bool) {
	// Prepare the response without disclosing data until lookup succeeds.
	ref := req.GetRef()
	resp = &DexMessage{RequestId: req.GetRequestId(), IsResponse: true}

	// Reject a block request that cannot identify stored data.
	if ref == nil || ref.GetEmpty() {
		resp.Error = "empty block ref"
		return resp, false
	}

	// Check the local bucket before forwarding.
	found, err := s.lookupLocalBlock(ctx, ref)
	if err != nil {
		resp.Error = err.Error()
		return resp, false
	}
	if found != nil {
		resp.SetBlock(found)
		// Answer linked, with the refs and no data, only when asked: the
		// serve reads read the block from a linked file, after verifying the
		// file's bytes against ref, so the file still holds it.
		if found.GetLinked() && req.GetLinkedOk() {
			resp.Linked, resp.Data = true, nil
		}
		return resp, true
	}

	// Forward to other peers if hops remain.
	// Clamp to configured max so a malicious peer cannot amplify traffic.
	maxHops := s.c.cc.GetMaxForwardHops()
	hops := min(req.GetRemainingHops(), maxHops)
	if hops > 0 {
		found, err := s.c.forwardToPeers(ctx, ref, hops-1, s)
		if err != nil {
			resp.Error = relayError(err)
			return resp, false
		}
		if found != nil {
			resp.SetBlock(found)
		}
	}
	return resp, false
}

// relayError names the cause of a forwarded lookup's failure for the
// requester: a peer's error, no peer session, or a timeout. It leaves out the
// downstream peers' own error text, which the fanout joins, so the answer
// stays one short string whatever the fanout and the hop count.
func relayError(err error) string {
	switch {
	case errors.Is(err, ErrPeerError):
		return ErrPeerError.Error()
	case errors.Is(err, ErrNoPeerSession):
		return ErrNoPeerSession.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded.Error()
	default:
		return "forwarded lookup failed"
	}
}

// sendMessage queues a message for the session writer and waits for its write.
// A sender whose ctx ends before the write starts drops its message; a write
// in progress completes on the stream it already holds.
func (s *peerSession) sendMessage(ctx, requestCtx context.Context, resp *DexMessage, access RequestAccess, closeOnCancel bool) error {
	// Refuse a write that ctx already canceled.
	if err := ctx.Err(); err != nil {
		return err
	}
	return waitSend(s.queueSend(ctx, requestCtx, resp, access, closeOnCancel))
}

// queueSend queues a message for the session writer, starting a writer when
// none is draining the queue. waitSend waits for its write.
func (s *peerSession) queueSend(ctx, requestCtx context.Context, resp *DexMessage, access RequestAccess, closeOnCancel bool) *queuedSend {
	// Append the send, and start a writer when none is draining the queue.
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
	return q
}

// waitSend waits for the write of q, abandoning the message if its ctx ends
// before the write starts.
func waitSend(q *queuedSend) error {
	select {
	case err := <-q.done:
		return err
	case <-q.ctx.Done():
		if q.state.CompareAndSwap(sendQueued, sendAbandoned) {
			return q.ctx.Err()
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
			p.msg = deniedResponse(q.msg.GetRequestId(), err)
			p.closeOnCancel = true
		} else if err = context.Cause(q.requestCtx); err != nil {
			p.msg = deniedResponse(q.msg.GetRequestId(), err)
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
		stops = append(stops, context.AfterFunc(p.q.ctx, func() {
			s.closeWith(errDenialWriteBlocked)
		}))
	}

	// Interrupt an in-progress payload write if its requester loses authority.
	// requestCtx also ends when the requester cancels the request or the
	// session closes, and access derives from the same request, so Check fails
	// then too; only a revocation of a live request closes the stream.
	if access := p.q.access; p.msg.GetFound() && access != nil {
		stops = append(stops, context.AfterFunc(p.q.requestCtx, func() {
			if p.q.ctx.Err() == nil && access.Check() != nil {
				s.closeWith(errAccessRevokedInWrite)
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

// lookupLocalBlock looks up a block and its refs on this side alone, as
// localReader reads. Returns nil when the block is not found.
func (s *peerSession) lookupLocalBlock(ctx context.Context, ref *block.BlockRef) (*DexMessage, error) {
	// Read the one block as a subtree walk reads its blocks.
	r := &localReader{s: s}
	defer r.release()
	var out [1]*block.StoredBlock
	if err := r.read(ctx, []*block.BlockRef{ref}, out[:]); err != nil || out[0] == nil {
		return nil, err
	}
	return storedBlockMessage(out[0]), nil
}

// localReader reads blocks from this side alone: the local source first,
// since it reaches the tiers below the volumes, then the configured bucket
// lookup without asking peers, which it opens at its first miss and holds
// until release.
type localReader struct {
	s *peerSession
	// opened is set once the bucket lookup was opened.
	opened bool
	// lk is the bucket lookup, nil when none is configured.
	lk bucket_lookup.Lookup
	// rel releases the bucket lookup.
	rel func()
}

// read sets out[i] to the block of refs[i], or leaves it nil when this side
// lacks it. A block either source fails to read fails the read.
func (r *localReader) read(ctx context.Context, refs []*block.BlockRef, out []*block.StoredBlock) error {
	// Read the local source's batch.
	if store := r.s.c.getLocalStore(); store != nil {
		err := block.GetStoredBlockBatch(ctx, store, refs, func(i int, stored *block.StoredBlock, err error) error {
			out[i] = stored
			return err
		})
		if err != nil {
			return err
		}
	}

	// Read what it lacks from the bucket lookup.
	for i, ref := range refs {
		if out[i] != nil {
			continue
		}
		lk, err := r.lookup(ctx)
		if err != nil || lk == nil {
			return err
		}
		if out[i], err = lk.LookupStoredBlock(ctx, ref, bucket_lookup.WithLocalOnly()); err != nil {
			return err
		}
	}
	return nil
}

// lookup returns the bucket lookup, opening it on the first call.
func (r *localReader) lookup(ctx context.Context) (bucket_lookup.Lookup, error) {
	// Open the lookup once, keeping its release for release.
	if r.opened {
		return r.lk, nil
	}
	lkv, _, lkRel, err := bucket_lookup.ExBuildBucketLookup(ctx, r.s.c.b, false, r.s.c.cc.GetBucketId(), nil)
	if err != nil {
		return nil, err
	}
	r.opened, r.rel = true, lkRel.Release
	r.lk, err = lkv.GetLookup(ctx)
	return r.lk, err
}

// release releases the bucket lookup if it was opened.
func (r *localReader) release() {
	if r.rel != nil {
		r.rel()
	}
}

// storedBlockMessage builds the found response of a stored block, marked
// Linked when a linked file served it.
func storedBlockMessage(stored *block.StoredBlock) *DexMessage {
	return &DexMessage{
		Found:     true,
		Data:      stored.Data,
		Refs:      stored.Refs,
		RefsKnown: stored.RefsKnown,
		Linked:    stored.Linked,
	}
}

// subtreeAsk asks a block request for the blocks under its ref as well.
type subtreeAsk struct {
	// budget is the bytes of block data asked for under the ref.
	budget uint32
	// have, when set, reports which of refs the asker already holds, so the
	// peer walks none of them.
	have func(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
	// claim, when set, is called once the ref's block verified with blocks to
	// follow; the request reads them only when it returns true, so of a
	// fanout's peers one streams the subtree.
	claim func() bool
}

// requestBlock sends a block request and waits for the response. Returns the
// verified response, or nil when the peer does not have the block. With
// linkedOK the peer may answer linked, with the refs and no data, which is
// returned unverified; a linked answer to a request without linkedOK is an
// error, so an untrusted peer cannot stand in a linked answer for the data.
// With sub it also asks for the blocks under ref and returns those the peer
// streams, as readSubtree keeps them.
func (s *peerSession) requestBlock(ctx context.Context, ref *block.BlockRef, hops uint32, linkedOK bool, sub *subtreeAsk) (*DexMessage, []*block.PutBatchEntry, error) {
	// Reject block requests after the peer session closes.
	if s.closed.Load() {
		return nil, nil, errSessionClosed
	}

	// Build the request. Its answer may carry one block, and the budget's
	// data under it.
	id := s.nextID.Add(1)
	req := &DexMessage{
		RequestId:     id,
		Ref:           ref,
		RemainingHops: hops,
		LinkedOk:      linkedOK,
	}
	p := &pendingRequest{wake: make(chan struct{}, 1), limit: maxMessageSize}
	if sub != nil {
		req.SubtreeBytes = sub.budget
		p.limit += int(sub.budget)
	}

	// Register the pending request before sending it.
	s.mtx.Lock()
	if s.closed.Load() {
		s.mtx.Unlock()
		return nil, nil, errSessionClosed
	}
	s.pending[id] = p
	s.rate.begin(time.Now())
	s.mtx.Unlock()
	defer func() {
		s.mtx.Lock()
		delete(s.pending, id)
		if len(s.pending) == 0 {
			s.rate.stop()
		}
		s.mtx.Unlock()
	}()

	// Count the session's bytes toward the read while the request is
	// pending.
	if p := getReadProgress(ctx); p != nil {
		defer p.end(p.begin(&s.rate.received))
	}

	// Send the block request to the peer.
	if err := s.sendMsg(ctx, req); err != nil {
		return nil, nil, err
	}

	// A request left before its last response tells the peer to stop serving
	// it, so the peer does not spend its upload on responses nobody reads.
	// The cancel outlives ctx and ends with the session, whose close fails
	// the write.
	open := true
	defer func() {
		if open && !s.closed.Load() {
			sendCtx := context.WithoutCancel(ctx)
			go func() {
				_ = s.sendMsg(sendCtx, &DexMessage{RequestId: id, Cancel: true})
			}()
		}
	}()

	// Await and check the answer for ref.
	msgs, err := s.take(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	open = msgs[0].GetMore()
	found, err := checkAnswer(ref, msgs[0], linkedOK)
	if err != nil || found == nil || !open {
		return found, nil, err
	}

	// Read the blocks under a verified block with its refs, when this request
	// claims them.
	if sub == nil || found.GetLinked() || !found.GetRefsKnown() || (sub.claim != nil && !sub.claim()) {
		return found, nil, nil
	}
	var blocks []*block.PutBatchEntry
	blocks, open = s.readSubtree(ctx, id, p, sub, found.GetRefs(), msgs[1:])
	return found, blocks, nil
}

// checkAnswer checks a peer's response to a request for ref. It returns the
// response when the peer found the block, nil when it does not have it, and
// an error when the peer failed, denied the request, or answered with what
// ref does not name.
func checkAnswer(ref *block.BlockRef, resp *DexMessage, linkedOK bool) (*DexMessage, error) {
	// Report a refusal, a failure or a miss.
	if resp.GetDenied() {
		return nil, fmt.Errorf("%w: %s", ErrPeerDenied, resp.GetError())
	}
	if resp.GetError() != "" {
		return nil, fmt.Errorf("%w: %s", ErrPeerError, resp.GetError())
	}
	if !resp.GetFound() {
		return nil, nil
	}

	// Accept a linked answer only where it was asked for, with the refs a
	// copy needs to settle the block.
	if resp.GetLinked() {
		if !linkedOK {
			return nil, ErrUnaskedLinked
		}
		if !resp.GetRefsKnown() {
			return nil, block.ErrRefsUnknown
		}
		return resp, nil
	}

	// Verify returned block data before reporting success.
	if err := ref.VerifyData(resp.GetData(), true); err != nil {
		return nil, err
	}
	return resp, nil
}

// readSubtree reads the stream of blocks a peer sends under a requested ref
// whose block named refs, msgs first, until the last response, the request
// ends, or the stream breaks its contract. It keeps a block only under a ref
// a kept block, or the ref's own, named, verified against that ref, with its
// refs known, and drops a block it did not ask for, such as one sent before
// its prune arrived. After each batch of responses it prunes the newly named
// refs that sub.have reports held. It ends the stream at a block that fails
// its hash or once the data passes sub.budget. It returns the kept blocks and
// whether the request is still open.
func (s *peerSession) readSubtree(
	ctx context.Context,
	id uint32,
	p *pendingRequest,
	sub *subtreeAsk,
	refs []*block.BlockRef,
	msgs []*DexMessage,
) ([]*block.PutBatchEntry, bool) {
	// want maps the key of each named ref to the ref, or to nil once its
	// block is kept or it is pruned.
	want := make(map[string]*block.BlockRef)
	var named []*block.BlockRef
	var keys []string
	name := func(refs []*block.BlockRef) {
		for _, ref := range refs {
			key := ref.MapKey()
			if _, ok := want[key]; ok || key == "" {
				continue
			}
			want[key] = ref
			named, keys = append(named, ref), append(keys, key)
		}
	}
	name(refs)

	// Read the stream's batches of responses until it ends.
	var blocks []*block.PutBatchEntry
	bytes := 0
	for {
		// Keep each block asked for.
		for _, msg := range msgs {
			if !msg.GetMore() {
				return blocks, false
			}
			bytes += len(msg.GetData())
			if bytes > int(sub.budget) {
				return blocks, true
			}
			key := msg.GetRef().MapKey()
			ref := want[key]
			if ref == nil || !msg.GetFound() || msg.GetLinked() || !msg.GetRefsKnown() {
				continue
			}
			if ref.VerifyData(msg.GetData(), true) != nil {
				return blocks, true
			}
			want[key] = nil
			blocks = append(blocks, &block.PutBatchEntry{
				Ref:       ref,
				Data:      msg.GetData(),
				Refs:      msg.GetRefs(),
				RefsKnown: true,
			})
			name(msg.GetRefs())
		}

		// Prune the refs named since the last prune that the asker holds.
		if sub.have != nil && len(named) != 0 {
			s.pruneHeld(ctx, id, sub, want, named, keys)
		}
		named, keys = named[:0], keys[:0]

		// Take the next responses.
		var err error
		if msgs, err = s.take(ctx, p); err != nil {
			return blocks, true
		}
	}
}

// pruneHeld sends the peer a prune of the refs of named, keyed by keys, that
// sub.have reports held, and marks them no longer wanted.
func (s *peerSession) pruneHeld(ctx context.Context, id uint32, sub *subtreeAsk, want map[string]*block.BlockRef, named []*block.BlockRef, keys []string) {
	// Ask the asker's store which of the named refs it holds.
	held, err := sub.have(ctx, named)
	if err != nil {
		s.le.WithError(err).Debug("dex subtree existence check failed")
		return
	}

	// Stop wanting the held refs and tell the peer to walk none of them.
	var prune []*block.BlockRef
	for i, ok := range held {
		if ok {
			want[keys[i]] = nil
			prune = append(prune, named[i])
		}
	}
	if len(prune) != 0 {
		_ = s.sendMsg(ctx, &DexMessage{RequestId: id, Prune: true, Refs: prune})
	}
}

// sendMsg sends a message with write serialization. It returns the context
// error without writing when ctx ends before the write starts.
func (s *peerSession) sendMsg(ctx context.Context, msg *DexMessage) error {
	return s.sendMessage(ctx, ctx, msg, nil, false)
}

// close closes the session stream.
func (s *peerSession) close() {
	s.closeWith(errSessionClosedLocally)
}

// closeWith closes the session stream, recording reason as why it ended
// unless it already ended.
func (s *peerSession) closeWith(reason error) {
	s.setEndErr(reason)
	if s.closed.CompareAndSwap(false, true) {
		if s.sess != nil {
			s.sess.Close()
		}
		s.wakePending()
	}
}

// setEndErr records err as why the session ended unless a reason is recorded.
func (s *peerSession) setEndErr(err error) {
	s.endErr.CompareAndSwap(nil, &err)
}

// getEndErr returns why the session ended, or nil while it runs.
func (s *peerSession) getEndErr() error {
	if err := s.endErr.Load(); err != nil {
		return *err
	}
	return nil
}

// wakePending ends every pending request with errSessionClosed.
func (s *peerSession) wakePending() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for id, p := range s.pending {
		p.end = errSessionClosed
		p.signal()
		delete(s.pending, id)
	}
}
