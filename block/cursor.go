package block

import (
	"errors"

	"github.com/aperturerobotics/hydra/cid"
)

// Cursor tracks traversal of a block reference DAG structure with an associated
// Transaction. Manages interacting with block handles, the transaction cache,
// the decoder and marshaller, and the transformers.
type Cursor struct {
	// t is the transaction
	t *Transaction
	// pos is the current block handle
	pos *handle
}

// newCursor builds a new cursor.
func newCursor(t *Transaction, pos *handle) *Cursor {
	return &Cursor{t: t, pos: pos}
}

// SetRef sets a block reference to the handle at the cursor.
func (c *Cursor) SetRef(
	refID uint32,
	cursor *Cursor,
) {
	if cursor == c || cursor.pos == c.pos {
		return
	}
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	} else {
		if r, ok := c.pos.refHandles[refID]; ok {
			if tgt := r.target; tgt != nil {
				c.t.blockGraph.RemoveEdge(c.pos.nod.ID(), tgt.nod.ID())
			}
		}
	}

	c.t.blockGraph.SetEdge(c.t.blockGraph.NewEdge(c.pos.nod, cursor.pos.nod))
	cursor.pos.parent = &refHandle{
		id:     refID,
		src:    c.pos,
		target: cursor.pos,
	}
	c.pos.refHandles[refID] = cursor.pos.parent
	cursor.markDirty()
}

// FollowRef follows a block reference, returning a cursor pointing to the next
// block and enqueuing the block for fetching. Does not wait for the block to be
// fetched to return. If the reference is empty, will create a new block.
func (c *Cursor) FollowRef(
	refID uint32,
	blkRef *cid.BlockRef,
) *Cursor {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	return c.followRef(refID, blkRef)
}

// followRef implements followRef assuming the mutex is locked
func (c *Cursor) followRef(refID uint32, blkRef *cid.BlockRef) *Cursor {
	ref := c.pos.refHandles[refID]
	if ref == nil {
		src := c.pos
		bn := c.t.blockGraph.NewNode()
		blkHandle := &handle{
			nod: bn,
			ref: blkRef,
		}
		ref = &refHandle{
			id:     refID,
			src:    c.pos,
			target: blkHandle,
		}
		blkHandle.parent = ref
		c.t.blockGraph.AddNode(bn)
		c.t.blockGraph.SetEdge(
			c.t.blockGraph.NewEdge(
				src.nod,
				bn,
			),
		)
		c.t.blocks[bn.ID()] = blkHandle
		ref.target = blkHandle
		c.pos.refHandles[refID] = ref
	}

	return newCursor(c.t, ref.target)
}

// ClearRef clears a block reference.
// Noop if FollowRef has not been previously called with refid.
func (c *Cursor) ClearRef(refID uint32) {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		return
	}
	r, ok := c.pos.refHandles[refID]
	if !ok {
		return
	}
	delete(c.pos.refHandles, refID)
	if tgt := r.target; tgt != nil {
		c.t.blockGraph.RemoveEdge(c.pos.nod.ID(), tgt.nod.ID())
	}
}

// RemapRefID remaps an existing ref ID if it exists.
/* TODO
func (c *Cursor) RemapRefID(oldRefID, nextRefID uint32) (found bool) {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		return
	}
	oref, ok := c.pos.refHandles[oldRefID]
	if !ok {
		return
	}
	found = true
	c.pos.refHandles
}
*/

// Fetch fetches the block data into memory.
// Fetching is performed using a block lookup.
func (c *Cursor) Fetch() ([]byte, bool, error) {
	if c.pos.ref.GetEmpty() {
		return nil, false, nil
	}

	return c.t.bucket.GetBlock(c.pos.ref)
}

// Unmarshal fetches and unmarshals the data to a block.
// If already unmarshaled, returns existing data.
// Returns found, error
func (c *Cursor) Unmarshal(ctor func() Block) (Block, error) {
	if c == nil || c.t == nil {
		return nil, errors.New("nil cursor")
	}
	c.t.mtx.Lock()
	b := c.pos.blk
	c.t.mtx.Unlock()
	if b == nil {
		b = ctor()
		if b == nil {
			return nil, errors.New("block constructor returned nil")
		}
	} else {
		return b, nil
	}

	dat, ok, err := c.Fetch()
	if err != nil || !ok {
		return nil, err
	}

	if err := b.UnmarshalBlock(dat); err != nil {
		return nil, err
	}

	c.t.mtx.Lock()
	if c.pos.blk != nil {
		b = c.pos.blk
	} else {
		c.pos.blk = b
	}
	c.t.mtx.Unlock()
	return b, nil
}

// GetRef returns the current cursor reference.
func (c *Cursor) GetRef() *cid.BlockRef {
	return c.pos.ref
}

// SetPreWriteHook sets a hook for final transforms to the block.
func (c *Cursor) SetPreWriteHook(h func(b Block) error) {
	c.pos.blkPreWrite = h
}

// SetBlock sets a block at the location, and marks the block as dirty.
func (c *Cursor) SetBlock(b Block) {
	c.t.mtx.Lock()
	c.pos.blk = b
	c.markDirty()
	c.t.mtx.Unlock()
}

// GetBlockRefs returns cursors to all pending / not pending references.
// If the position blk is empty, returns an empty map.
func (c *Cursor) GetAllRefs() (map[uint32]*Cursor, error) {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	m := map[uint32]*Cursor{}
	if c.pos.blk == nil {
		return m, nil
	}
	blockRefs, err := c.pos.blk.GetBlockRefs()
	if err != nil {
		return nil, err
	}
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	// load all block refs to ref handles
	for refID, bref := range blockRefs {
		if bref == nil || bref.GetEmpty() {
			continue
		}
		if _, ok := c.pos.refHandles[refID]; ok {
			continue
		}
		m[refID] = c.followRef(refID, bref)
	}
	// priority: pending block refs
	for refID, refHandle := range c.pos.refHandles {
		if _, ok := m[refID]; ok {
			continue
		}
		if refHandle == nil || refHandle.target == nil {
			continue
		}
		m[refID] = newCursor(c.t, refHandle.target)
	}
	return m, nil
}

// markDirty assumes c.t.mtx is locked
func (c *Cursor) markDirty() {
	c.t.dirty = true
	if c.pos != nil {
		c.pos.dirty = true
		for {
			ref := c.pos.parent
			if ref == nil || ref.src.dirty {
				break
			}
			ref.src.dirty = true
		}
	}
}
