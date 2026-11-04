package block

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/net/hash"
)

// refHoldsOn is set once a caller enables holds, as local eviction does when
// it starts. Until then NewRefHold returns nil and reads record nothing, so a
// process that never evicts pays one atomic load per block read.
var refHoldsOn atomic.Bool

// EnableRefHolds makes NewRefHold return holds that record the blocks read
// in their contexts. Holds made before it stay nil.
func EnableRefHolds() {
	refHoldsOn.Store(true)
}

// refKeyLen is the length of a held key: a prefix of the block hash. Two
// blocks sharing a prefix are both held while either is.
const refKeyLen = 16

// refKey identifies a held block by a prefix of its hash.
type refKey [refKeyLen]byte

// newRefKey returns the key of the block with hash h, false without one.
func newRefKey(h *hash.Hash) (refKey, bool) {
	// Copy the hash prefix, skipping an empty hash.
	var key refKey
	b := h.GetHash()
	if len(b) == 0 {
		return key, false
	}
	copy(key[:], b)
	return key, true
}

// refCountShards spreads the held counts over locks, since every live view
// adds the blocks it reads.
const refCountShards = 64

// refCountShard counts the holds of the keys in one shard.
type refCountShard struct {
	mtx    sync.Mutex
	counts map[refKey]int32
}

// heldRefs counts, for each held key, the live holds that hold it.
var heldRefs [refCountShards]refCountShard

// shardOf returns the shard counting key.
func shardOf(key refKey) *refCountShard {
	return &heldRefs[key[0]%refCountShards]
}

// Held reports whether a live hold holds the block with hash h.
func Held(h *hash.Hash) bool {
	// Look the key's count up in its shard.
	key, ok := newRefKey(h)
	if !ok {
		return false
	}
	s := shardOf(key)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.counts[key] > 0
}

// RefHold holds the blocks a long-lived decoded value was built from, such as
// a view that serves from memory for hours, so eviction keeps their local
// copies while the value lives: after a restart its rebuild reads them from
// the disk instead of the remote. Reads through a Cursor in a context from
// WithRefHold add their blocks. A hold is released by Release, or once nothing
// references it. A nil hold holds nothing. Methods are safe for concurrent
// use.
type RefHold struct {
	s *refHoldState
}

// refHoldState is a hold's keys, apart from the RefHold so the cleanup that
// releases them can run once the RefHold is unreachable.
type refHoldState struct {
	// mtx guards the fields below and orders every count change of the
	// hold's keys, so a release never runs between an add's insert and its
	// count.
	mtx sync.Mutex
	// keys are the keys the hold counts.
	keys map[refKey]struct{}
	// kept are the holds this one keeps reachable.
	kept []*RefHold
	// released is set once the keys are uncounted.
	released bool
}

// NewRefHold returns a hold, or nil before EnableRefHolds.
func NewRefHold() *RefHold {
	// Hold nothing until holds are enabled.
	if !refHoldsOn.Load() {
		return nil
	}

	// Release the keys once the hold is unreachable.
	s := &refHoldState{keys: make(map[refKey]struct{})}
	h := &RefHold{s: s}
	runtime.AddCleanup(h, (*refHoldState).release, s)
	return h
}

// Len returns the number of blocks h holds.
func (h *RefHold) Len() int {
	if h == nil {
		return 0
	}
	h.s.mtx.Lock()
	defer h.s.mtx.Unlock()
	return len(h.s.keys)
}

// add holds the block ref points at.
func (h *RefHold) add(ref *BlockRef) {
	// Skip a nil hold and a ref without a hash.
	if h == nil {
		return
	}
	key, ok := newRefKey(ref.GetHash())
	if !ok {
		return
	}

	// Record the key once per hold, unless the hold was released.
	h.s.mtx.Lock()
	defer h.s.mtx.Unlock()
	if _, ok := h.s.keys[key]; ok || h.s.released {
		return
	}
	h.s.keys[key] = struct{}{}

	// Count it in its shard while the hold's lock orders it before release.
	s := shardOf(key)
	s.mtx.Lock()
	if s.counts == nil {
		s.counts = make(map[refKey]int32)
	}
	s.counts[key]++
	s.mtx.Unlock()
}

// Keep keeps child reachable, so it holds its blocks, while h is unreleased:
// a view keeps the holds of the cached values it was built from.
func (h *RefHold) Keep(child *RefHold) {
	if h == nil || child == nil || h == child {
		return
	}
	h.s.mtx.Lock()
	defer h.s.mtx.Unlock()
	if !h.s.released {
		h.s.kept = append(h.s.kept, child)
	}
}

// Release stops h holding its blocks and the holds it kept. A later read in
// its context holds nothing.
func (h *RefHold) Release() {
	if h != nil {
		h.s.release()
	}
}

// release uncounts the keys once.
func (s *refHoldState) release() {
	// Mark the hold released, once.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.released {
		return
	}
	s.released = true

	// Uncount each key, dropping the ones no hold holds.
	for key := range s.keys {
		sh := shardOf(key)
		sh.mtx.Lock()
		if sh.counts[key]--; sh.counts[key] <= 0 {
			delete(sh.counts, key)
		}
		sh.mtx.Unlock()
	}
	s.keys, s.kept = nil, nil
}

// refHoldKey carries the RefHold of a context.
type refHoldKey struct{}

// WithRefHold returns ctx whose Cursor reads add their blocks to h. A nil h
// returns ctx.
func WithRefHold(ctx context.Context, h *RefHold) context.Context {
	if h == nil {
		return ctx
	}
	return context.WithValue(ctx, refHoldKey{}, h)
}

// RefHoldFromContext returns the hold of ctx, nil without one.
func RefHoldFromContext(ctx context.Context) *RefHold {
	if !refHoldsOn.Load() {
		return nil
	}
	h, _ := ctx.Value(refHoldKey{}).(*RefHold)
	return h
}

// holdRead adds the block ref points at to the hold of ctx, if any.
func holdRead(ctx context.Context, ref *BlockRef) {
	if refHoldsOn.Load() {
		RefHoldFromContext(ctx).add(ref)
	}
}
