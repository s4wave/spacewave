package dex_solicit

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

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

	sess   *stream_packet.Session
	nextID atomic.Uint32
	closed atomic.Bool

	// mtx guards pending map and serializes SendMsg writes.
	mtx     sync.Mutex
	pending map[uint32]chan *DexMessage
}

// run starts the session, reading messages in a loop and dispatching
// responses to pending requests or handling incoming requests.
func (s *peerSession) run(ctx context.Context) {
	defer func() {
		s.closed.Store(true)
		s.sess.Close()
		// Wake any pending requests.
		s.mtx.Lock()
		for id, ch := range s.pending {
			select {
			case ch <- nil:
			default:
			}
			delete(s.pending, id)
		}
		s.mtx.Unlock()
	}()

	for {
		var msg DexMessage
		if err := s.sess.RecvMsg(&msg); err != nil {
			if err != io.EOF && ctx.Err() == nil {
				s.le.WithError(err).Debug("dex session read error")
			}
			return
		}

		if msg.GetIsResponse() {
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

	// Check local store first.
	data, found, err := s.lookupLocalBlock(ctx, ref)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	if found {
		resp.Found = true
		resp.Data = data
		return
	}

	// Forward to other peers if hops remain.
	// Clamp to configured max so a malicious peer cannot amplify traffic.
	maxHops := s.c.cc.GetMaxForwardHops()
	hops := min(req.GetRemainingHops(), maxHops)
	if hops > 0 {
		data, found = s.c.forwardToPeers(ctx, ref, hops-1, s)
		if found {
			resp.Found = true
			resp.Data = data
		}
	}
}

// lookupLocalBlock looks up a block in the local bucket store only.
func (s *peerSession) lookupLocalBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	lkv, _, lkRel, err := bucket_lookup.ExBuildBucketLookup(ctx, s.c.b, false, s.c.cc.GetBucketId(), nil)
	if err != nil {
		return nil, false, err
	}
	defer lkRel.Release()

	lk, err := lkv.GetLookup(ctx)
	if err != nil {
		return nil, false, err
	}
	if lk == nil {
		return nil, false, nil
	}

	return lk.LookupBlock(ctx, ref, bucket_lookup.WithLocalOnly())
}

// requestBlock sends a block request and waits for the response.
func (s *peerSession) requestBlock(ctx context.Context, ref *block.BlockRef, hops uint32) ([]byte, bool, error) {
	if s.closed.Load() {
		return nil, false, errors.New("session closed")
	}

	id := s.nextID.Add(1)
	ch := make(chan *DexMessage, 1)
	s.mtx.Lock()
	s.pending[id] = ch
	s.mtx.Unlock()
	defer func() {
		s.mtx.Lock()
		delete(s.pending, id)
		s.mtx.Unlock()
	}()

	req := &DexMessage{
		RequestId:     id,
		Ref:           ref,
		RemainingHops: hops,
	}
	if err := s.sendMsg(req); err != nil {
		return nil, false, err
	}

	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return nil, false, errors.New("session closed")
		}
		if resp.GetError() != "" {
			return nil, false, errors.New(resp.GetError())
		}
		return resp.GetData(), resp.GetFound(), nil
	}
}

// sendMsg sends a message with write serialization.
func (s *peerSession) sendMsg(msg *DexMessage) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.sess.SendMsg(msg)
}

// close closes the session stream.
func (s *peerSession) close() {
	if s.closed.CompareAndSwap(false, true) {
		if s.sess != nil {
			s.sess.Close()
		}
	}
}
