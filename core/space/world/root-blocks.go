package space_world

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// repeatedRefShift is the bit shift that separates a repeated ref's field
// number from its index. The root blocks identify their refs by id so a generic
// walker, such as block.ExtractBlockRefs on a block the type registry decoded,
// sees the same children the writer listed. A single ref uses its proto field
// number. The entry i of a repeated field uses repeatedRefID(field, i).
const repeatedRefShift = 24

// repeatedRefID is the ref id of entry idx of the repeated ref field field.
func repeatedRefID(field uint32, idx int) uint32 {
	return field<<repeatedRefShift | uint32(idx) //nolint:gosec
}

// repeatedRefIndex returns the entry index of id within the repeated field
// field of n entries, or false when id identifies no entry of that field.
func repeatedRefIndex(id, field uint32, n int) (int, bool) {
	if id>>repeatedRefShift != field {
		return 0, false
	}
	idx := int(id & (1<<repeatedRefShift - 1))
	return idx, idx < n
}

// addRepeatedRefs adds the entries of the repeated ref field field to m.
func addRepeatedRefs(m map[uint32]*block.BlockRef, field uint32, refs []*block.BlockRef) {
	for i, ref := range refs {
		m[repeatedRefID(field, i)] = ref
	}
}

// ApplyBlockRef applies a ref change with a field id.
func (h *HeldRoots) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 2 {
		h.Prev = ptr
		return nil
	}
	if idx, ok := repeatedRefIndex(id, 1, len(h.GetRoots())); ok {
		h.Roots[idx] = ptr
		return nil
	}
	return errors.Errorf("unknown held roots ref id %d", id)
}

// GetBlockRefs returns the held roots and the earlier block.
func (h *HeldRoots) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, len(h.GetRoots())+1)
	addRepeatedRefs(m, 1, h.GetRoots())
	m[2] = h.GetPrev()
	return m, nil
}

// GetBlockRefCtor returns the earlier block's constructor. A held root's type
// depends on the root that holds the block, so a held root has no constructor.
func (h *HeldRoots) GetBlockRefCtor(id uint32) block.Ctor {
	if id == 2 {
		return func() block.Block { return &HeldRoots{} }
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
func (s *ReplaySpan) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 3 {
		s.Prev = ptr
		return nil
	}
	if idx, ok := repeatedRefIndex(id, 1, len(s.GetWorlds())); ok {
		s.Worlds[idx] = ptr
		return nil
	}
	if idx, ok := repeatedRefIndex(id, 2, len(s.GetPayloads())); ok {
		s.Payloads[idx] = ptr
		return nil
	}
	return errors.Errorf("unknown replay span ref id %d", id)
}

// GetBlockRefs returns the span's Worlds, payloads and earlier span.
func (s *ReplaySpan) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	// Record the Worlds, then the payloads, then the earlier span.
	m := make(map[uint32]*block.BlockRef, len(s.GetWorlds())+len(s.GetPayloads())+1)
	addRepeatedRefs(m, 1, s.GetWorlds())
	addRepeatedRefs(m, 2, s.GetPayloads())
	m[3] = s.GetPrev()
	return m, nil
}

// GetBlockRefCtor returns the earlier span's constructor.
func (s *ReplaySpan) GetBlockRefCtor(id uint32) block.Ctor {
	if id == 3 {
		return func() block.Block { return &ReplaySpan{} }
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
func (r *RetainedRootSet) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if idx, ok := repeatedRefIndex(id, 1, len(r.GetRoots())); ok {
		r.Roots[idx].RootRef = ptr
		return nil
	}
	return errors.Errorf("unknown retained root set ref id %d", id)
}

// GetBlockRefs returns the retained roots.
func (r *RetainedRootSet) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, len(r.GetRoots()))
	for i, root := range r.GetRoots() {
		m[repeatedRefID(1, i)] = root.GetRootRef()
	}
	return m, nil
}

// GetBlockRefCtor returns nil: a retained root is a World root.
func (r *RetainedRootSet) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

// _ is a type assertion
var (
	_ block.BlockWithRefs = (*HeldRoots)(nil)
	_ block.BlockWithRefs = (*ReplaySpan)(nil)
	_ block.BlockWithRefs = (*RetainedRootSet)(nil)
)
