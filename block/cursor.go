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
	c.pos.refHandles[refID] = &refHandle{
		id:     refID,
		src:    c.pos,
		target: cursor.pos,
	}
}

// FollowRef follows a block reference, returning a cursor pointing to the next
// block and enqueuing the block for fetching. Does not wait for the block to be
// fetched to return. If the reference is empty, will create a new block.
func (c *Cursor) FollowRef(
	refID uint32,
	blkRef *cid.BlockRef,
) (*Cursor, error) {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
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

	return newCursor(c.t, ref.target), nil
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
	c.t.mtx.Lock()
	b := c.pos.blk
	c.t.mtx.Unlock()
	if b == nil {
		b = ctor()
		if b == nil {
			return nil, errors.New("block constructor returned nil")
		}
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

// SetPreWriteHook sets a hook for final transforms to the block.
func (c *Cursor) SetPreWriteHook(h func(b Block) error) {
	c.pos.blkPreWrite = h
}

// SetBlock sets a block at the location, and marks the block as dirty.
func (c *Cursor) SetBlock(b Block) {
	c.t.mtx.Lock()
	c.t.dirty = true
	c.pos.blk = b
	c.pos.dirty = true
	for {
		ref := c.pos.parent
		if ref == nil || ref.src.dirty {
			break
		}
		ref.src.dirty = true
	}
	c.t.mtx.Unlock()
}
