package dex_solicit

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

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

	// mtx guards pending map and serializes SendMsg writes.
	mtx     sync.Mutex
	pending map[uint32]chan *DexMessage
}

func newPeerSession(
	c *Controller,
	le *logrus.Entry,
	ms link.MountedStream,
	onExit func(),
) *peerSession {
	s := &peerSession{
		c:       c,
		le:      le,
		ms:      ms,
		sess:    stream_packet.NewSession(ms.GetStream(), maxMessageSize),
		onExit:  onExit,
		pending: make(map[uint32]chan *DexMessage),
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
// responses to pending requests or handling incoming requests.
func (s *peerSession) run(ctx context.Context) {
	// Mark the session closed and wake pending requests on exit.
	defer func() {
		s.closed.Store(true)
		s.sess.Close()
		s.wakePending()
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

		if msg.GetIsResponse() {
			s.c.recordTransfer(s.ms.GetPeerID().String(), 0, len(msg.GetData()))
			s.mtx.Lock()
			ch, ok := s.pending[msg.GetRequestId()]
			if ok {
				delete(s.pending, msg.GetRequestId())
			}
			s.mtx.Unlock()
			if ok {
				ch <- &msg
			}
			continue
		}

		// Incoming request: handle locally and send response.
		go s.handleRequest(ctx, &msg)
	}
}

// handleRequest handles an incoming block request and sends a response.
func (s *peerSession) handleRequest(ctx context.Context, req *DexMessage) {
	// Prepare a response that is sent when request handling finishes.
	ref := req.GetRef()
	resp := &DexMessage{
		RequestId:  req.GetRequestId(),
		IsResponse: true,
	}
	defer func() {
		if err := s.sendMsg(resp); err != nil {
			s.le.WithError(err).Debug("dex session send error")
		}
	}()

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
		if found := s.c.forwardToPeers(ctx, ref, hops-1, s); found != nil {
			resp.SetBlock(found)
		}
	}
}

// lookupLocalBlock looks up a block and its refs in the local bucket store
// only. Returns nil when the block is not found.
func (s *peerSession) lookupLocalBlock(ctx context.Context, ref *block.BlockRef) (*DexMessage, error) {
	lkv, _, lkRel, err := bucket_lookup.ExBuildBucketLookup(ctx, s.c.b, false, s.c.cc.GetBucketId(), nil)
	if err != nil {
		return nil, err
	}
	defer lkRel.Release()

	lk, err := lkv.GetLookup(ctx)
	if err != nil || lk == nil {
		return nil, err
	}

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
	if err := s.sendMsg(req); err != nil {
		return nil, err
	}

	// Await the response or request cancellation.
	select {
	case <-ctx.Done():
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

// sendMsg sends a message with write serialization.
func (s *peerSession) sendMsg(msg *DexMessage) error {
	s.mtx.Lock()
	err := s.sess.SendMsg(msg)
	s.mtx.Unlock()
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
