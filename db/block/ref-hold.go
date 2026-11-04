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

// refKeyLen is the length of a held key: a prefix of the block hash, as local
// eviction keys its reads. Two blocks sharing a prefix are both held while
// either is.
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

// refShardBits is the number of low slot bits naming a shard.
const refShardBits = 6

// refShards spreads the held table over locks, since every live view adds the
// blocks it reads.
const refShards = 1 << refShardBits

// refShard is one shard of the held table: one slot per distinct held block,
// shared by every hold of it, in flat arrays indexed by slot position. A slot
// is freed when its count reaches zero and reused by the next new key.
type refShard struct {
	mtx sync.Mutex
	// index maps a held key to its slot position.
	index map[refKey]uint32
	// keys, counts and stamps are indexed by slot position: the slot's key,
	// the number of increments live holds made to it, and the id of the
	// hold that last incremented it.
	keys   []refKey
	counts []int32
	stamps []uint64
	// free lists the positions of freed slots.
	free []uint32
}

// heldRefs is the held table: for each distinct held block, one count of the
// live holds' increments.
var heldRefs [refShards]refShard

// refShardOf returns the shard holding key.
func refShardOf(key refKey) uint32 {
	return uint32(key[0]) & (refShards - 1)
}

// incLocked counts one increment of key by the hold with id, returning the
// slot id, or false when that hold made the slot's last increment already.
// The caller holds s.mtx.
func (s *refShard) incLocked(shard uint32, key refKey, id uint64) (uint32, bool) {
	// Find the key's slot, taking a freed one or a new one for a new key.
	pos, ok := s.index[key]
	if !ok {
		if s.index == nil {
			s.index = make(map[refKey]uint32)
		}
		if n := len(s.free); n != 0 {
			pos, s.free = s.free[n-1], s.free[:n-1]
			s.keys[pos] = key
		} else {
			pos = uint32(len(s.keys)) // #nosec G115 -- a shard holds far fewer than 1<<26 blocks
			s.keys = append(s.keys, key)
			s.counts = append(s.counts, 0)
			s.stamps = append(s.stamps, 0)
		}
		s.index[key] = pos
	}

	// Skip a repeat by the hold that incremented it last, then count it.
	if s.stamps[pos] == id {
		return 0, false
	}
	s.stamps[pos] = id
	s.counts[pos]++
	return pos<<refShardBits | shard, true
}

// dec undoes one increment of the slot with id slot, freeing the slot once no
// increment is left.
func dec(slot uint32) {
	// Decrement the slot in its shard.
	s := &heldRefs[slot&(refShards-1)]
	pos := slot >> refShardBits
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.counts[pos]--; s.counts[pos] > 0 {
		return
	}

	// Free the slot once no hold counts it.
	delete(s.index, s.keys[pos])
	s.counts[pos], s.stamps[pos] = 0, 0
	s.free = append(s.free, pos)
}

// Held reports whether a live hold holds the block with hash h.
func Held(h *hash.Hash) bool {
	// Look the key up in its shard; a slot exists while its count is nonzero.
	key, ok := newRefKey(h)
	if !ok {
		return false
	}
	s := &heldRefs[refShardOf(key)]
	s.mtx.Lock()
	defer s.mtx.Unlock()
	_, ok = s.index[key]
	return ok
}

// refHoldIDs numbers holds for the held table's stamps. Zero is no hold.
var refHoldIDs atomic.Uint64

// RefHold holds the blocks a long-lived decoded value was built from, such as
// a view that serves from memory for hours, so eviction keeps their local
// copies while the value lives: after a restart its rebuild reads them from
// the disk instead of the remote. Reads through a Cursor in a context from
// WithRefHold add their blocks. A hold is released by Release, or once nothing
// references it. A nil hold holds nothing. Methods are safe for concurrent
// use.
//
// A hold keeps only the slot ids of the held table it incremented, four bytes
// per block, so a block read by many views takes one slot. Repeated reads of
// a block by one hold count once unless another hold incremented the block in
// between; the extra increment is undone with the rest.
type RefHold struct {
	s *refHoldState
}

// refHoldState is a hold's slots, apart from the RefHold so the cleanup that
// releases them can run once the RefHold is unreachable.
type refHoldState struct {
	// id stamps the slots the hold increments.
	id uint64
	// mtx guards the fields below and orders every increment of the hold
	// before its release, so a release never misses one.
	mtx sync.Mutex
	// slots are the held table slots the hold incremented.
	slots []uint32
	// kept are the holds this one keeps reachable.
	kept []*RefHold
	// released is set once the slots are decremented.
	released bool
}

// NewRefHold returns a hold, or nil before EnableRefHolds.
func NewRefHold() *RefHold {
	// Hold nothing until holds are enabled.
	if !refHoldsOn.Load() {
		return nil
	}

	// Release the slots once the hold is unreachable.
	s := &refHoldState{id: refHoldIDs.Add(1)}
	h := &RefHold{s: s}
	runtime.AddCleanup(h, (*refHoldState).release, s)
	return h
}

// Len returns the number of slot increments h holds: the blocks it holds,
// plus the repeats another hold's reads interleaved.
func (h *RefHold) Len() int {
	if h == nil {
		return 0
	}
	h.s.mtx.Lock()
	defer h.s.mtx.Unlock()
	return len(h.s.slots)
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

	// Take the hold's lock, so a release sees the increment, unless the hold
	// was released.
	h.s.mtx.Lock()
	defer h.s.mtx.Unlock()
	if h.s.released {
		return
	}

	// Increment the block's slot and record it once per increment.
	shard := refShardOf(key)
	s := &heldRefs[shard]
	s.mtx.Lock()
	slot, ok := s.incLocked(shard, key, h.s.id)
	s.mtx.Unlock()
	if ok {
		h.s.slots = append(h.s.slots, slot)
	}
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

// release decrements the slots once.
func (s *refHoldState) release() {
	// Mark the hold released, once, taking its slots.
	s.mtx.Lock()
	if s.released {
		s.mtx.Unlock()
		return
	}
	s.released = true
	slots := s.slots
	s.slots, s.kept = nil, nil
	s.mtx.Unlock()

	// Undo each increment; no add counts into a released hold.
	for _, slot := range slots {
		dec(slot)
	}
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

// RefHoldStats describes the held table.
type RefHoldStats struct {
	// Blocks is the number of distinct held blocks.
	Blocks int
	// Slots is the number of slots allocated, held or free.
	Slots int
}

// ReadRefHoldStats returns the held table's size.
func ReadRefHoldStats() RefHoldStats {
	var st RefHoldStats
	for i := range heldRefs {
		s := &heldRefs[i]
		s.mtx.Lock()
		st.Blocks += len(s.index)
		st.Slots += len(s.keys)
		s.mtx.Unlock()
	}
	return st
}
