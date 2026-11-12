package block

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
)

// PendingStore keeps the blocks put through it in memory and stores them only
// when a named root moves onto a block that reaches them. A writer whose
// record of a change is not its blocks, such as a SharedObject whose
// operation log rebuilds a World, writes every intermediate tree through it,
// and only the trees a root names reach the store: a page written and
// replaced between two named roots never enters the Volume's graph.
//
// Reads serve a kept block first, then the inner store. SetRoots adds to its
// entries every kept block a moved root reaches, so the root and its tree land
// in one transaction, and drops them from memory once the inner store holds
// them. Sweep drops the kept blocks no live root reaches. A put past the
// memory bound goes to the inner store, or to the stage a writer opened
// through OpenStage, which holds it until its writer's root does.
//
// Invariants:
//   - A kept block carries its complete child list, and each child is kept or
//     was put to the inner store, so a walk from a root through kept blocks
//     never reads the inner store to find a child. The writer holds each
//     stored child while a kept block names it, under a root, a stage or a
//     pin of its own.
//   - A put past the bound stores the kept blocks below the entry with it and
//     keeps them, so a block the inner store holds names only blocks it holds
//     and the kept Worlds still find their pages here.
//   - A kept block leaves memory only once the inner store holds it under a
//     root moved in the same transaction, or when a sweep finds that no root
//     it is given, no block an open stage or pin holds, and no block kept
//     after the sweep's mark reaches it. The sweep checks the holds and marks
//     again under the lock it drops with, and walks from those its first
//     walk missed.
//   - A pin on a kept block holds its kept tree in memory, with no write, by
//     holding the root's key alone, so a pin and its release cost the same
//     at any tree size.
//   - A flushed block is stored under its Flush's stage until the Flush is
//     released, and no Flush releases while a SetRoots or Flush that left out
//     a flushed block is open, so the move that names it names a block the
//     inner store holds.
type PendingStore struct {
	// StoreOps is the inner store. Methods PendingStore does not override
	// pass through to it.
	StoreOps

	// mtx guards kept, keptBytes, held and each open stage's keys. It is never
	// held across I/O. Reads of kept blocks and the walk SetRoots makes under
	// a moved root share it, so a read does not wait for a walk.
	mtx sync.RWMutex
	// kept holds the blocks put and not yet stored, by reference.
	kept map[string]*PutBatchEntry
	// held counts, by key, the open stages that put each kept block. Sweep
	// keeps a held block and what it reaches: its writer has not named its
	// root yet.
	held map[string]int
	// flushed counts, by key, the open Flushes that stored each kept block
	// under their stage. SetRoots puts no flushed block again.
	flushed map[string]int
	// skipping counts the SetRoots in flight and the open Flushes that left
	// out a block an open Flush stored, and deferred holds the flush stage
	// releases that wait for the last of them to end. A walk that holds mtx
	// shared adds to skipping, so it is atomic; it is read and lowered under
	// mtx held exclusive.
	skipping atomic.Int64
	deferred []func()
	// keptAt is the put sequence at which each kept block was kept. A sweep
	// drops only blocks kept before its mark.
	keptAt map[string]uint64
	// seq counts the puts that kept a block.
	seq uint64
	// keptBytes is the total data length of kept.
	keptBytes int
	// maxBytes bounds keptBytes. A put past it goes to the inner store.
	maxBytes int
}

// NewPendingStore returns a PendingStore over inner that keeps at most
// maxBytes of blocks in memory.
func NewPendingStore(inner StoreOps, maxBytes int) *PendingStore {
	return &PendingStore{
		StoreOps: inner,
		kept:     make(map[string]*PutBatchEntry),
		held:     make(map[string]int),
		flushed:  make(map[string]int),
		keptAt:   make(map[string]uint64),
		maxBytes: maxBytes,
	}
}

// Full reports whether the kept blocks reached half the bound, so the writer
// names a tree before more puts must go to the inner store.
func (s *PendingStore) Full() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.keptBytes >= s.maxBytes/2
}

// KeptBytes returns the total data length of the kept blocks.
func (s *PendingStore) KeptBytes() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.keptBytes
}

// GetSupportedFeatures reports the inner store's features, and that the store
// buffers its own puts: they are readable from memory at once, so a World
// engine that defers durability writes to it directly.
func (s *PendingStore) GetSupportedFeatures() StoreFeature {
	return s.StoreOps.GetSupportedFeatures() | StoreFeatureSelfBuffered
}

// BeginReadOperation returns the store itself, so a scoped read still serves
// the kept blocks.
func (s *PendingStore) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock keeps a block whose refs are known, and puts any other to the
// inner store.
func (s *PendingStore) PutBlock(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	// Hand a block without its child list to the inner store.
	if !opts.GetRefsKnown() || opts.GetSync() {
		return s.StoreOps.PutBlock(ctx, data, opts)
	}

	// Keep it.
	ref, err := BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	existed, _, err := s.put(ctx, s.StoreOps, nil, []*PutBatchEntry{{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: true}})
	if err != nil {
		return nil, false, err
	}
	return ref, existed[0], nil
}

// PutBlockBatch keeps each entry with a complete child list while the bound
// allows, and puts the rest to the inner store. A kept entry reports whether
// it was kept already.
func (s *PendingStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	existed, _, err := s.put(ctx, s.StoreOps, nil, entries)
	return existed, err
}

// put keeps the entries it can and puts the rest to dst, reporting whether
// it put any. Each entry kept through hold, when set, is held until the hold
// is released. The kept blocks an entry past the bound names are stored with
// it and stay kept, so the Worlds that name them still find them here.
// Tombstones are refused: a kept block may be reached by any kept World.
func (s *PendingStore) put(ctx context.Context, dst StoreOps, hold *pendingHold, entries []*PutBatchEntry) ([]bool, bool, error) {
	// Refuse tombstones before keeping anything.
	for _, e := range entries {
		if e.Tombstone {
			return nil, false, ErrPendingTombstone
		}
	}

	// Keep each entry with a child list that fits, in order.
	out := make([]bool, len(entries))
	var rest []*PutBatchEntry
	var restIdx []int
	s.mtx.Lock()
	for i, e := range entries {
		key := pendingKey(e.Ref)
		switch {
		case s.kept[key] != nil:
			out[i] = true
			s.seq++
			s.keptAt[key] = s.seq
			s.holdLocked(hold, key)
		case e.RefsKnown && s.keptBytes+len(e.Data) <= s.maxBytes:
			s.seq++
			s.kept[key], s.keptAt[key] = e, s.seq
			s.keptBytes += len(e.Data)
			s.holdLocked(hold, key)
		default:
			rest, restIdx = append(rest, e), append(restIdx, i)
		}
	}
	s.mtx.Unlock()

	// Gather the kept blocks the rest name: a block the inner store holds
	// names only blocks it holds.
	if len(rest) == 0 {
		return out, false, nil
	}
	var children []*BlockRef
	for _, e := range rest {
		children = append(children, e.Refs...)
	}
	tree := s.reach(children)
	s.checkHoles(ctx, "put", slices.Concat(tree, rest))

	// Put them first, then the rest, keeping the tree.
	found, err := dst.PutBlockBatch(ctx, slices.Concat(tree, rest))
	if err != nil {
		return nil, true, err
	}
	for j, i := range restIdx {
		out[i] = found[len(tree)+j]
	}
	return out, true, nil
}

// pendingKey returns the key of ref in kept: its hash type and digest. A read
// of every block passes through lookup, so the key avoids the base58 encoding
// of MarshalString.
func pendingKey(ref *BlockRef) string {
	h := ref.GetHash()
	return string(rune(h.GetHashType())) + string(h.GetHash())
}

// pendingHold is a set of kept blocks a stage or a pin holds: Sweep keeps
// each, and what it reaches, until the hold is released.
type pendingHold struct {
	// keys are the held blocks. Guarded by the PendingStore's mtx.
	keys map[string]struct{}
}

// newPendingHold returns an empty hold.
func newPendingHold() *pendingHold {
	return &pendingHold{keys: make(map[string]struct{})}
}

// holdLocked adds the kept block key to h, once per hold. A nil h holds
// nothing. The caller holds mtx.
func (s *PendingStore) holdLocked(h *pendingHold, key string) {
	if h == nil || h.keys == nil {
		return
	}
	if _, ok := h.keys[key]; ok {
		return
	}
	h.keys[key] = struct{}{}
	s.held[key]++
}

// release drops h's holds. Each block it held is stamped as kept now, so a
// sweep that took its mark while the hold stood keeps it: that sweep may
// have gathered its live roots before the holder named the block, such as a
// writer that admits its candidate and ends its stage mid-sweep.
func (s *PendingStore) release(h *pendingHold) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for key := range h.keys {
		if s.held[key]--; s.held[key] <= 0 {
			delete(s.held, key)
		}
		if s.kept[key] != nil {
			s.seq++
			s.keptAt[key] = s.seq
		}
	}
	h.keys = nil
}

// lookup returns the kept entry of ref, or nil.
func (s *PendingStore) lookup(ref *BlockRef) *PutBatchEntry {
	// Skip the key when nothing is kept.
	if ref.GetEmpty() {
		return nil
	}
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if len(s.kept) == 0 {
		return nil
	}

	// Find the kept entry.
	return s.kept[pendingKey(ref)]
}

// GetBlock serves a kept block, or reads the inner store. A kept block's
// decoded form may enter the shared pool: Sweep reports each block it forgets.
func (s *PendingStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	gen := DurableReadGeneration()
	if e := s.lookup(ref); e != nil {
		MarkDurableRead(ctx, ref, gen)
		return e.Data, true, nil
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock serves a kept block with its child list, or reads the inner
// store.
func (s *PendingStore) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	gen := DurableReadGeneration()
	if e := s.lookup(ref); e != nil {
		MarkDurableRead(ctx, ref, gen)
		return &StoredBlock{Data: e.Data, Refs: e.Refs, RefsKnown: true}, nil
	}
	return s.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlockExists reports a kept block, or asks the inner store.
func (s *PendingStore) GetBlockExists(ctx context.Context, ref *BlockRef) (bool, error) {
	if s.lookup(ref) != nil {
		return true, nil
	}
	return s.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports the kept blocks and asks the inner store about
// the rest.
func (s *PendingStore) GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	// Answer the kept blocks.
	out := make([]bool, len(refs))
	var rest []*BlockRef
	var restIdx []int
	for i, ref := range refs {
		if s.lookup(ref) != nil {
			out[i] = true
			continue
		}
		rest, restIdx = append(rest, ref), append(restIdx, i)
	}

	// Ask the inner store about the rest.
	if len(rest) == 0 {
		return out, nil
	}
	found, err := s.StoreOps.GetBlockExistsBatch(ctx, rest)
	if err != nil {
		return nil, err
	}
	for j, i := range restIdx {
		out[i] = found[j]
	}
	return out, nil
}

// StatBlock reports a kept block's size, or asks the inner store.
func (s *PendingStore) StatBlock(ctx context.Context, ref *BlockRef) (*BlockStat, error) {
	if e := s.lookup(ref); e != nil {
		return &BlockStat{Ref: ref, Size: int64(len(e.Data))}, nil
	}
	return s.StoreOps.StatBlock(ctx, ref)
}

// RmBlock drops a kept block and removes it from the inner store.
func (s *PendingStore) RmBlock(ctx context.Context, ref *BlockRef) error {
	s.drop([]string{pendingKey(ref)})
	return s.StoreOps.RmBlock(ctx, ref)
}

// SetRoots stores entries and every kept block a moved root or an entry
// reaches, and moves the roots, in one inner SetRoots, then drops the stored
// blocks from memory. A root may name an entry, such as a new block naming
// several kept trees, so the walk starts from each entry's children too. A
// block an open Flush stored is not put again: its stage holds it until the
// move names it. The walk still descends through it, since a block kept
// below it after the flush's walk may be one no store holds.
func (s *PendingStore) SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error {
	// Gather the kept trees under the moved roots and the entries.
	refs := make([]*BlockRef, 0, len(roots))
	for _, root := range roots {
		refs = append(refs, root.Ref)
	}
	for _, e := range entries {
		refs = append(refs, e.Refs...)
	}
	tree, keys, _ := s.reachSliced(refs, false)
	s.checkHoles(ctx, "SetRoots", slices.Concat(tree, entries))
	s.checkRootHoles(ctx, slices.Concat(tree, entries), roots)

	// Store them with the entries, but those an open flush stored, and move
	// the roots.
	batch, landed := s.unflushed(tree, keys)
	err := s.StoreOps.SetRoots(ctx, slices.Concat(entries, batch), roots)
	landed()
	if err != nil {
		return err
	}
	s.dropSliced(keys)
	return nil
}

// reach returns the kept blocks the roots reach, children before parents.
func (s *PendingStore) reach(roots []*BlockRef) []*PutBatchEntry {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.reachLocked(roots, make(map[string]struct{}))
}

// reachLocked is reach for a caller holding mtx, shared or exclusive. It
// adds each block it returns to seen, and neither returns nor descends into a
// block seen holds already.
func (s *PendingStore) reachLocked(roots []*BlockRef, seen map[string]struct{}) []*PutBatchEntry {
	// Walk the kept blocks from each root, depth first, emitting each block
	// after its kept children.
	var out []*PutBatchEntry
	type frame struct {
		key   string
		e     *PutBatchEntry
		child int
	}
	for _, root := range roots {
		if root.GetEmpty() {
			continue
		}
		key := pendingKey(root)
		e := s.kept[key]
		if _, ok := seen[key]; ok || e == nil {
			continue
		}
		seen[key] = struct{}{}
		stack := []frame{{key: key, e: e}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.child < len(top.e.Refs) {
				ref := top.e.Refs[top.child]
				top.child++
				ckey := pendingKey(ref)
				if _, ok := seen[ckey]; ok {
					continue
				}
				if ce := s.kept[ckey]; ce != nil {
					seen[ckey] = struct{}{}
					stack = append(stack, frame{key: ckey, e: ce})
				}
				continue
			}
			out = append(out, top.e)
			stack = stack[:len(stack)-1]
		}
	}
	return out
}

// drop forgets the kept blocks with keys.
func (s *PendingStore) drop(keys []string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, key := range keys {
		if e := s.kept[key]; e != nil {
			s.keptBytes -= len(e.Data)
			delete(s.kept, key)
			delete(s.keptAt, key)
		}
	}
}

// SweepMark returns the mark a caller takes before it gathers the live roots
// of a Sweep. Blocks kept after the mark belong to roots the caller may not
// have seen, so the Sweep keeps them.
func (s *PendingStore) SweepMark() uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.seq
}

// Sweep drops the kept blocks kept at or before mark that none of live
// reaches, keeping every block a stage or pin holds, every block kept after
// mark, and what each of those reaches.
func (s *PendingStore) Sweep(live []*BlockRef, mark uint64) {
	// Mark what the live roots and the held blocks reach.
	reached := make(map[string]struct{})
	s.mtx.RLock()
	for key := range s.held {
		if e := s.kept[key]; e != nil {
			live = append(live, e.Ref)
		}
	}
	s.reachLocked(live, reached)
	s.mtx.RUnlock()

	// Gather the blocks no mark reached, and those of them a stage or pin
	// holds now or that were kept after mark: a hold taken since the held
	// set was read, or a release that stamped its blocks, protects the tree
	// below it, and a pin holds only its root.
	s.mtx.Lock()
	var unreached []string
	var since []*BlockRef
	for key, e := range s.kept {
		if _, ok := reached[key]; ok {
			continue
		}
		if s.held[key] != 0 || s.keptAt[key] > mark {
			since = append(since, e.Ref)
		}
		unreached = append(unreached, key)
	}

	// Drop the rest once the walk from those marks what they reach.
	s.reachLocked(since, reached)
	var forgot []*BlockRef
	for _, key := range unreached {
		if _, ok := reached[key]; !ok {
			e := s.kept[key]
			s.keptBytes -= len(e.Data)
			delete(s.kept, key)
			delete(s.keptAt, key)
			recordSwept(key)
			forgot = append(forgot, e.Ref)
		}
	}
	s.mtx.Unlock()

	// Report them, so the decoded pool stops serving a block no store holds.
	for _, ref := range forgot {
		InvalidateRemovedBlock(ref)
	}
}

// HoldsRoots reports whether the inner store holds pins and stages.
func (s *PendingStore) HoldsRoots() bool {
	return HoldsRoots(s.StoreOps)
}

// PinRoot holds ref until release. A kept ref is held in memory with the
// kept tree below it, with no write, by a hold on its key alone: Sweep keeps
// what a held block reaches. Any other ref is pinned in the inner store.
func (s *PendingStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	// Hold a kept tree in memory.
	key := pendingKey(ref)
	s.mtx.Lock()
	if s.kept[key] != nil {
		h := newPendingHold()
		s.holdLocked(h, key)
		s.mtx.Unlock()
		var once sync.Once
		return func() { once.Do(func() { s.release(h) }) }, nil
	}
	s.mtx.Unlock()

	// Pin a stored one in the inner store.
	return PinRoot(ctx, s.StoreOps, ref)
}

// OpenStage opens a stage on the inner store whose writes are kept here while
// the bound allows and go to the stage past it.
func (s *PendingStore) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	stage, release, err := OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, nil, err
	}
	ps := &pendingStage{PendingStore: s, stage: stage, hold: newPendingHold()}
	return ps, func() {
		s.release(ps.hold)
		release()
	}, nil
}

// OpenOverflowStage opens a stage of the inner store for the puts past the
// bound, as OpenStage does, but holds none of the kept blocks it puts
// against Sweep. It suits a writer that passes Sweep every tree it still
// needs and can compute the others again: a kept page it replaced leaves
// memory at the next sweep, not at the release, and only the stored ones
// wait for the stage.
func (s *PendingStore) OpenOverflowStage(ctx context.Context) (*OverflowStage, error) {
	stage, release, err := OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, err
	}
	return &OverflowStage{pendingStage: &pendingStage{PendingStore: s, stage: stage}, release: release}, nil
}

// ReleaseRoots forwards staging release to the inner store.
func (s *PendingStore) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	return ReleaseRoots(ctx, s.StoreOps, refs)
}

// MarkRootsComplete forwards completion proofs to the inner store.
func (s *PendingStore) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	return MarkRootsComplete(ctx, s.StoreOps, roots)
}

// RootsComplete forwards completion reads to the inner store.
func (s *PendingStore) RootsComplete(ctx context.Context, roots []*BlockRef) ([]bool, error) {
	if !s.HoldsRoots() {
		return nil, errors.New("pending store: inner store holds no roots")
	}
	return RootsComplete(ctx, s.StoreOps, roots)
}

// HoldCompleteRoots reports no proofs: a PendingStore is not a stage. Its
// stages hold proved roots through the inner stage.
func (s *PendingStore) HoldCompleteRoots(_ context.Context, refs []*BlockRef) ([]bool, error) {
	return make([]bool, len(refs)), nil
}

// _ is a type assertion
var (
	_ StoreOps    = (*PendingStore)(nil)
	_ LeasedRoots = (*PendingStore)(nil)
	_ RootProofs  = (*PendingStore)(nil)
)
