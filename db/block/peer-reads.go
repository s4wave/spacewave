package block

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// peerReadsContextKey holds the PeerReads of a read that may not ask peers.
type peerReadsContextKey struct{}

// peerReadFetchers bounds the peer fetches one PeerReads runs at once.
const peerReadFetchers = 8

// PeerFetch reads one block from peers with no lock held, waiting as long as
// an ordinary peer read does. It returns nil when every peer lacks the block.
type PeerFetch func(ctx context.Context) (*StoredBlock, error)

// PeerReads defers the peer reads of a caller that may not wait for a peer,
// such as a World write body under the writer lock. A read under
// WithPeerReads that every tier but the peers missed returns ErrUnavailable at
// once and records the block with a fetch for it. The caller runs Fetch with
// no lock held, then runs again under the same PeerReads, whose reads of the
// fetched blocks answer from it. One PeerReads lives across a caller's
// attempts and is safe for concurrent use.
type PeerReads struct {
	mtx sync.Mutex
	// pending are the blocks deferred and not fetched yet, by marshaled ref.
	pending map[string]PeerFetch
	// fetched are the blocks Fetch read, by marshaled ref.
	fetched map[string]*StoredBlock
	// rounds counts the Fetch calls.
	rounds int
}

// NewPeerReads constructs an empty PeerReads.
func NewPeerReads() *PeerReads {
	return &PeerReads{}
}

// WithPeerReads returns a context whose reads that would ask peers defer to
// r instead. A nil r returns ctx with peer reads allowed again.
func WithPeerReads(ctx context.Context, r *PeerReads) context.Context {
	return context.WithValue(ctx, peerReadsContextKey{}, r)
}

// GetPeerReads returns the PeerReads WithPeerReads attached to ctx, or nil
// when reads under ctx may ask peers.
func GetPeerReads(ctx context.Context) *PeerReads {
	r, _ := ctx.Value(peerReadsContextKey{}).(*PeerReads)
	return r
}

// Lookup returns the block Fetch read for ref, or nil.
func (r *PeerReads) Lookup(ref *BlockRef) *StoredBlock {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.fetched[ref.MarshalString()]
}

// Defer records ref as a block to fetch with fetch, unless it is recorded.
func (r *PeerReads) Defer(ref *BlockRef, fetch PeerFetch) {
	// Skip a block already fetched.
	key := ref.MarshalString()
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if _, ok := r.fetched[key]; ok {
		return
	}

	// Record its fetch once.
	if r.pending == nil {
		r.pending = make(map[string]PeerFetch)
	}
	if _, ok := r.pending[key]; !ok {
		r.pending[key] = fetch
	}
}

// Pending reports whether a read deferred a block not fetched yet.
func (r *PeerReads) Pending() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return len(r.pending) != 0
}

// Rounds returns the number of Fetch calls so far.
func (r *PeerReads) Rounds() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.rounds
}

// Fetch reads every pending block from peers, a few at a time, and keeps
// them for Lookup. It reports whether every block was found. A block no peer
// holds, or a fetch that failed, is dropped from pending, and the first such
// failure is returned; ok is false then too.
func (r *PeerReads) Fetch(ctx context.Context) (ok bool, err error) {
	// Take the pending fetches.
	r.mtx.Lock()
	pending := r.pending
	r.pending = nil
	r.rounds++
	r.mtx.Unlock()

	// Fetch them with a bounded fanout.
	var eg errgroup.Group
	eg.SetLimit(peerReadFetchers)
	var mtx sync.Mutex
	ok = true
	for key, fetch := range pending {
		eg.Go(func() error {
			// Fetch the block, and count a miss or failure.
			stored, err := fetch(ctx)
			mtx.Lock()
			defer mtx.Unlock()
			if err != nil || stored == nil {
				ok = false
				return err
			}

			// Keep it for Lookup.
			r.mtx.Lock()
			if r.fetched == nil {
				r.fetched = make(map[string]*StoredBlock)
			}
			r.fetched[key] = stored
			r.mtx.Unlock()
			return nil
		})
	}
	err = eg.Wait()
	return ok && err == nil, err
}
