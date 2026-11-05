package block

// handle contains working state for a block.
type handle struct {
	// parents are the traversed references targeting this position.
	parents []*refHandle
	// ref is the base block reference, or nil before assignment.
	ref *BlockRef
	// isSubBlock indicates if this is a sub-block.
	isSubBlock bool
	// refHandles indexes traversed outgoing references by their field IDs.
	refHandles map[uint32]*refHandle
	// dirty indicates the block has been changed.
	dirty bool
	// moved indicates only the parents changed. The block still matches ref,
	// so writes reuse ref instead of encoding the block again.
	moved bool

	// blk is the decoded block or sub-block pointer, when known.
	blk any
	// shared marks blk as a decoded-block cache instance that other
	// transactions may hold. Call ownBlock before changing blk in place.
	shared bool
	// blkPreWrite transforms the decoded block before writing.
	blkPreWrite func(b any) error
}

// Clone clones the handle object.
// The traversed references are cleared; the decoded block is shared.
func (h *handle) Clone() *handle {
	return &handle{
		ref:         h.ref.Clone(),
		isSubBlock:  h.isSubBlock,
		refHandles:  make(map[uint32]*refHandle),
		dirty:       h.dirty,
		moved:       h.moved,
		blk:         h.blk,
		shared:      h.shared,
		blkPreWrite: h.blkPreWrite,
	}
}

// ownBlock replaces a shared decoded block with a private clone, so the
// position may change it in place.
func (h *handle) ownBlock() error {
	// A private block may already change in place.
	if !h.shared {
		return nil
	}

	// Replace the shared instance with its clone.
	blk, err := CloneBlock(h.blk)
	if err != nil {
		return err
	}
	h.blk, h.shared = blk, false
	return nil
}

// refHandle is a block ref handle.
type refHandle struct {
	// id is the reference field identifier assigned by the block type.
	id uint32
	// src is the referencing block handle.
	src *handle
	// target is the referenced block handle.
	target *handle
}

// addParent adds a parent removing any existing parents from the source.
// The edge must target h.
func (h *handle) addParent(rh *refHandle) {
	// Replace the existing relation from this source before attaching its edge.
	h.removeParent(rh.src)
	h.parents = append(h.parents, rh)
}

// removeParent removes parent references with oh as the source.
func (h *handle) removeParent(oh *handle) []*refHandle {
	// The source's outgoing references can disprove membership without scanning
	// a heavily shared target. Cursor mutations retain both ends until removal.
	if oh != nil && oh.refHandles != nil && len(oh.refHandles) < len(h.parents) {
		found := false
		for _, ref := range oh.refHandles {
			if ref.target == h {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	// Remove matching incoming edges without retaining their backing pointers.
	var removed []*refHandle
	for i := 0; i < len(h.parents); i++ {
		parent := h.parents[i]
		if parent.src == oh {
			removed = append(removed, parent)
			h.parents[i] = h.parents[len(h.parents)-1]
			h.parents[len(h.parents)-1] = nil
			h.parents = h.parents[:len(h.parents)-1]
			i--
		}
	}
	return removed
}
