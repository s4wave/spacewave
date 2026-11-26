package lookup_concurrent

import (
	"container/list"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// fillHoldBytes bounds the block data a fillHold keeps: four network answers
// to a subtree read, so the answers to a few reads in flight wait together.
const fillHoldBytes = 4 * subtreeLookupBytes

// fillHold holds the blocks the network sent ahead of a walk that no bucket
// kept, each until the walk reads it, in a bounded amount of memory.
//
// A bucket that keeps liveness drops a cache fill whose refs its source only
// listed, as block.ErrRefsUnknown describes, so the blocks a peer sends under
// a subtree read cannot wait in the bucket for the walk. They wait here: the
// walk's read of one takes it without asking the network again, and fills the
// bucket with the refs it decodes. The oldest blocks leave when newer ones
// need their room. Every block held came from the network, which serves it
// again, so losing one costs only its next fetch.
type fillHold struct {
	// limit bounds bytes.
	limit int

	// mtx guards the fields below.
	mtx sync.Mutex
	// blocks maps a held block's ref key to its element in order.
	blocks map[string]*list.Element
	// order holds the blocks, oldest first.
	order list.List
	// bytes is the block data held.
	bytes int
}

// heldFill is one block a fillHold holds.
type heldFill struct {
	// key is the block's ref key, its key in blocks.
	key string
	// stored is the block with the refs its source listed.
	stored *block.StoredBlock
}

// newFillHold builds a hold bounded at limit bytes of block data.
func newFillHold(limit int) *fillHold {
	return &fillHold{limit: limit, blocks: make(map[string]*list.Element)}
}

// put holds stored as the block of ref, unless it is held already or is
// larger than the bound, and drops the oldest blocks past the bound. The hold
// keeps stored's slices, which the caller no longer changes.
func (h *fillHold) put(ref *block.BlockRef, stored *block.StoredBlock) {
	// Skip a block the hold could never keep.
	size := len(stored.Data)
	if size > h.limit {
		return
	}

	// Hold it as the newest block.
	key := ref.MapKey()
	h.mtx.Lock()
	defer h.mtx.Unlock()
	if _, ok := h.blocks[key]; ok {
		return
	}
	h.blocks[key] = h.order.PushBack(&heldFill{key: key, stored: stored})
	h.bytes += size

	// Drop the oldest blocks past the bound.
	for h.bytes > h.limit {
		h.remove(h.order.Front())
	}
}

// take returns the block held for ref and stops holding it, or returns nil.
// The caller owns the block: a reader may decode its bytes in place.
func (h *fillHold) take(ref *block.BlockRef) *block.StoredBlock {
	// Find the block and hand it over.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	el, ok := h.blocks[ref.MapKey()]
	if !ok {
		return nil
	}
	return h.remove(el)
}

// holds reports whether the hold has the block of ref.
func (h *fillHold) holds(ref *block.BlockRef) bool {
	// Look the block up without taking it.
	h.mtx.Lock()
	defer h.mtx.Unlock()
	_, ok := h.blocks[ref.MapKey()]
	return ok
}

// remove stops holding el's block and returns it. The caller holds mtx.
func (h *fillHold) remove(el *list.Element) *block.StoredBlock {
	held := h.order.Remove(el).(*heldFill)
	delete(h.blocks, held.key)
	h.bytes -= len(held.stored.Data)
	return held.stored
}
