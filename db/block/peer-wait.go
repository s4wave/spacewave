package block

import (
	"context"
	"sync"
)

type peerWaitContextKey struct{}

// PeerWait records the first block a read reported unavailable instead of
// waiting for a peer to connect and serve it.
type PeerWait struct {
	mtx sync.Mutex
	ref *BlockRef
}

// WithoutPeerWait returns a context whose block reads report ErrUnavailable
// instead of waiting for a peer to connect, and the record of the first block
// such a read skipped.
func WithoutPeerWait(ctx context.Context) (context.Context, *PeerWait) {
	wait := &PeerWait{}
	return context.WithValue(ctx, peerWaitContextKey{}, wait), wait
}

// GetPeerWait returns the record WithoutPeerWait attached to ctx, or nil when
// reads under ctx may wait for a peer.
func GetPeerWait(ctx context.Context) *PeerWait {
	wait, _ := ctx.Value(peerWaitContextKey{}).(*PeerWait)
	return wait
}

// Skip records ref as skipped unless an earlier read already recorded a block.
func (w *PeerWait) Skip(ref *BlockRef) {
	w.mtx.Lock()
	if w.ref == nil {
		w.ref = ref
	}
	w.mtx.Unlock()
}

// GetRef returns the first skipped block, or nil when no read skipped one.
func (w *PeerWait) GetRef() *BlockRef {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.ref
}
