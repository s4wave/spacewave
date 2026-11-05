package block

import (
	"context"
	"slices"
	"sync"

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
//     never reads the inner store to find a child.
//   - A block put to the inner store names no kept block: a put past the
//     bound puts the kept blocks below it first, in the same batch.
//   - A kept block leaves memory only once the inner store holds it under a
//     root moved in the same transaction, or when no root Sweep is given
//     reaches it.
//   - A pin on a kept block stores the tree below it before the pin, so the
//     collector never sees a pinned block without its children.
type PendingStore struct {
	// StoreOps is the inner store. Methods PendingStore does not override
	// pass through to it.
	StoreOps

	// mtx guards kept and keptBytes. It is never held across I/O.
	mtx sync.Mutex
	// kept holds the blocks put and not yet stored, by reference.
	kept map[string]*PutBatchEntry
	// keptBytes is the total data length of kept.
	keptBytes int
	// maxBytes bounds keptBytes. A put past it goes to the inner store.
	maxBytes int
}

// NewPendingStore returns a PendingStore over inner that keeps at most
// maxBytes of blocks in memory.
func NewPendingStore(inner StoreOps, maxBytes int) *PendingStore {
	return &PendingStore{StoreOps: inner, kept: make(map[string]*PutBatchEntry), maxBytes: maxBytes}
}

// Full reports whether the kept blocks reached half the bound, so the writer
// names a tree before more puts must go to the inner store.
func (s *PendingStore) Full() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.keptBytes >= s.maxBytes/2
}

// KeptBytes returns the total data length of the kept blocks.
func (s *PendingStore) KeptBytes() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
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
	existed, err := s.put(ctx, s.StoreOps, []*PutBatchEntry{{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: true}})
	if err != nil {
		return nil, false, err
	}
	return ref, existed[0], nil
}

// PutBlockBatch keeps each entry with a complete child list while the bound
// allows, and puts the rest to the inner store. A kept entry reports whether
// it was kept already.
func (s *PendingStore) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	return s.put(ctx, s.StoreOps, entries)
}

// put keeps the entries it can and puts the rest to dst.
func (s *PendingStore) put(ctx context.Context, dst StoreOps, entries []*PutBatchEntry) ([]bool, error) {
	// Keep each entry with a child list that fits, in order.
	out := make([]bool, len(entries))
	var rest []*PutBatchEntry
	var restIdx []int
	s.mtx.Lock()
	for i, e := range entries {
		key := e.Ref.MarshalString()
		switch {
		case e.Tombstone:
			if prev := s.kept[key]; prev != nil {
				s.keptBytes -= len(prev.Data)
				delete(s.kept, key)
			}
			rest, restIdx = append(rest, e), append(restIdx, i)
		case s.kept[key] != nil:
			out[i] = true
		case e.RefsKnown && s.keptBytes+len(e.Data) <= s.maxBytes:
			s.kept[key] = e
			s.keptBytes += len(e.Data)
		default:
			rest, restIdx = append(rest, e), append(restIdx, i)
		}
	}
	s.mtx.Unlock()

	// Gather the kept blocks the rest name: a block the inner store holds
	// names only blocks it holds.
	if len(rest) == 0 {
		return out, nil
	}
	var children []*BlockRef
	for _, e := range rest {
		children = append(children, e.Refs...)
	}
	tree, keys := s.reach(children)

	// Put them first, then the rest.
	found, err := dst.PutBlockBatch(ctx, slices.Concat(tree, rest))
	if err != nil {
		return nil, err
	}
	s.drop(keys)
	for j, i := range restIdx {
		out[i] = found[len(tree)+j]
	}
	return out, nil
}

// lookup returns the kept entry of ref, or nil.
func (s *PendingStore) lookup(ref *BlockRef) *PutBatchEntry {
	if ref.GetEmpty() {
		return nil
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.kept[ref.MarshalString()]
}

// GetBlock serves a kept block, or reads the inner store.
func (s *PendingStore) GetBlock(ctx context.Context, ref *BlockRef) ([]byte, bool, error) {
	if e := s.lookup(ref); e != nil {
		return e.Data, true, nil
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock serves a kept block with its child list, or reads the inner
// store.
func (s *PendingStore) GetStoredBlock(ctx context.Context, ref *BlockRef) (*StoredBlock, error) {
	if e := s.lookup(ref); e != nil {
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
	s.drop([]string{ref.MarshalString()})
	return s.StoreOps.RmBlock(ctx, ref)
}

// SetRoots stores entries and every kept block a moved root or an entry
// reaches, and moves the roots, in one inner SetRoots, then drops the stored
// blocks from memory. A root may name an entry, such as a new block naming
// several kept trees, so the walk starts from each entry's children too.
func (s *PendingStore) SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error {
	// Gather the kept trees under the moved roots and the entries.
	refs := make([]*BlockRef, 0, len(roots))
	for _, root := range roots {
		refs = append(refs, root.Ref)
	}
	for _, e := range entries {
		refs = append(refs, e.Refs...)
	}
	tree, keys := s.reach(refs)

	// Store them with the entries and move the roots.
	if err := s.StoreOps.SetRoots(ctx, slices.Concat(entries, tree), roots); err != nil {
		return err
	}
	s.drop(keys)
	return nil
}

// reach returns the kept blocks the roots reach, children before parents, and
// their keys.
func (s *PendingStore) reach(roots []*BlockRef) ([]*PutBatchEntry, []string) {
	// Walk the kept blocks from each root, depth first, emitting each block
	// after its kept children.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var out []*PutBatchEntry
	var keys []string
	seen := make(map[string]struct{})
	type frame struct {
		key   string
		e     *PutBatchEntry
		child int
	}
	for _, root := range roots {
		if root.GetEmpty() {
			continue
		}
		key := root.MarshalString()
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
				ckey := ref.MarshalString()
				if _, ok := seen[ckey]; ok {
					continue
				}
				if ce := s.kept[ckey]; ce != nil {
					seen[ckey] = struct{}{}
					stack = append(stack, frame{key: ckey, e: ce})
				}
				continue
			}
			out, keys = append(out, top.e), append(keys, top.key)
			stack = stack[:len(stack)-1]
		}
	}
	return out, keys
}

// drop forgets the kept blocks with keys.
func (s *PendingStore) drop(keys []string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, key := range keys {
		if e := s.kept[key]; e != nil {
			s.keptBytes -= len(e.Data)
			delete(s.kept, key)
		}
	}
}

// Sweep drops the kept blocks none of live reaches.
func (s *PendingStore) Sweep(live []*BlockRef) {
	// Mark what the live roots reach.
	_, keys := s.reach(live)
	reached := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		reached[key] = struct{}{}
	}

	// Drop the rest.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for key, e := range s.kept {
		if _, ok := reached[key]; !ok {
			s.keptBytes -= len(e.Data)
			delete(s.kept, key)
		}
	}
}

// SupportsRootRetention reports whether the inner store holds pins and stages.
func (s *PendingStore) SupportsRootRetention() bool {
	return SupportsRootRetention(s.StoreOps)
}

// PinRoot stores the kept tree below ref, then pins ref in the inner store.
func (s *PendingStore) PinRoot(ctx context.Context, ref *BlockRef) (func(), error) {
	// Store the kept tree first, so the pin holds a stored tree.
	tree, keys := s.reach([]*BlockRef{ref})
	if len(tree) != 0 {
		if err := s.StoreOps.SetRoots(ctx, tree, nil); err != nil {
			return nil, err
		}
	}

	// Pin it, then forget what the store holds now.
	release, err := PinRoot(ctx, s.StoreOps, ref)
	if err != nil {
		return nil, err
	}
	s.drop(keys)
	return release, nil
}

// OpenStage opens a stage on the inner store whose writes are kept here while
// the bound allows and go to the stage past it.
func (s *PendingStore) OpenStage(ctx context.Context) (StoreOps, func(), error) {
	stage, release, err := OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, nil, err
	}
	return &pendingStage{PendingStore: s, stage: stage}, release, nil
}

// ReleaseRoots forwards staging release to the inner store.
func (s *PendingStore) ReleaseRoots(ctx context.Context, refs []*BlockRef) error {
	return ReleaseRoots(ctx, s.StoreOps, refs)
}

// MarkRootsComplete forwards completion proofs to the inner store.
func (s *PendingStore) MarkRootsComplete(ctx context.Context, roots []*BlockRef) error {
	if !s.SupportsRootRetention() {
		return nil
	}
	return s.StoreOps.(RootRetainer).MarkRootsComplete(ctx, roots)
}

// RootsComplete forwards completion reads to the inner store.
func (s *PendingStore) RootsComplete(ctx context.Context, roots []*BlockRef) ([]bool, error) {
	if !s.SupportsRootRetention() {
		return nil, errors.New("pending store: inner store holds no roots")
	}
	return s.StoreOps.(RootRetainer).RootsComplete(ctx, roots)
}

// _ is a type assertion
var (
	_ StoreOps     = (*PendingStore)(nil)
	_ RootRetainer = (*PendingStore)(nil)
)
