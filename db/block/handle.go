package block

import "strconv"

// handle contains working state for a block.
type handle struct {
	// Node is the base graph node.
	Node GraphNode

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

	// blk is the decoded block or sub-block pointer, when known.
	blk any
	// blkPreWrite transforms the decoded block before writing.
	blkPreWrite func(b any) error
}

// ID returns the node identifier of the underlying graph node.
func (h *handle) ID() int64 {
	return h.Node.ID()
}

// Clone clones the handle object.
// The graph node and traversed references are cleared; the decoded block is shared.
func (h *handle) Clone() *handle {
	return &handle{
		ref:         h.ref.Clone(),
		isSubBlock:  h.isSubBlock,
		refHandles:  make(map[uint32]*refHandle),
		dirty:       h.dirty,
		blk:         h.blk,
		blkPreWrite: h.blkPreWrite,
	}
}

// DOTID returns a DOT node ID.
//
// An ID is one of the following:
//
//   - a string of alphabetic ([a-zA-Z\x80-\xff]) characters, underscores ('_').
//     digits ([0-9]), not beginning with a digit.
//   - a numeral [-]?(.[0-9]+ | [0-9]+(.[0-9]*)?).
//   - a double-quoted string ("...") possibly containing escaped quotes (\").
//   - an HTML string (<...>).
func (h *handle) DOTID() string {
	// Identify a sub-block by its parent and reference field.
	if h.isSubBlock {
		var parentid string
		var subBlockId uint32
		if len(h.parents) != 0 && h.parents[0].src != nil {
			parentid = h.parents[0].src.DOTID()
			subBlockId = h.parents[0].id
		}
		return parentid + "@" + strconv.FormatUint(uint64(subBlockId), 10)
	}

	// Identify a standalone block by its content reference.
	return h.ref.MarshalString()
}

// Attributes returns the decoded block's graph attributes, when available.
func (h *handle) Attributes() []BlockGraphAttribute {
	var res []BlockGraphAttribute
	if h.blk != nil {
		attrs, ok := h.blk.(BlockWithAttributes)
		if ok {
			res = append(res, attrs.GetBlockGraphAttributes()...)
		}
	}
	return res
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

// From returns the from node of the edge.
func (r *refHandle) From() GraphNode {
	return r.src
}

// To returns the to node of the edge.
func (r *refHandle) To() GraphNode {
	return r.target
}

// ReversedEdge returns an edge with the source and target swapped.
func (r *refHandle) ReversedEdge() GraphEdge {
	return &refHandle{src: r.target, target: r.src}
}

// addParent adds a parent removing any existing parents from the source.
func (h *handle) addParent(rh *refHandle) []*refHandle {
	// Reject edges addressed to another block.
	if rh.target != h {
		return nil
	}

	// Replace the existing relation from this source before attaching its edge.
	out := h.removeParent(rh.src)
	h.parents = append(h.parents, rh)
	return out
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

// _ is a type assertion
var (
	_ GraphNode = (*handle)(nil)
	_ GraphEdge = (*refHandle)(nil)
)
