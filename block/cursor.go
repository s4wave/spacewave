package block

import (
	"context"
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

// FollowRef follows a block reference, returning a cursor pointing to the next
// block and enqueuing the block for fetching. Does not wait for the block to be
// fetched to return. If the reference is empty, will immediately return nil, nil.
func (c *Cursor) FollowRef(
	ctx context.Context,
	refID uint32,
	blkRef *cid.BlockRef,
) (*Cursor, error) {
	if blkRef.GetEmpty() {
		return nil, nil
	}

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

// Fetch fetches the block data into memory.
// Fetching is performed using a block lookup.
func (c *Cursor) Fetch(ctx context.Context) ([]byte, bool, error) {
	return c.t.bucket.GetBlock(c.pos.ref)
}

// Unmarshal fetches and unmarshals the data to a block.
// If already unmarshaled, returns existing data.
// Returns found, error
func (c *Cursor) Unmarshal(ctx context.Context, ctor func() Block) (Block, error) {
	c.t.mtx.Lock()
	b := c.pos.blk
	c.t.mtx.Unlock()
	if b == nil {
		b = ctor()
		if b == nil {
			return nil, errors.New("block constructor returned nil")
		}
	}

	dat, ok, err := c.Fetch(ctx)
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

// SetBlock sets a block at the location, and marks the block as dirty.
func (c *Cursor) SetBlock(b Block) {
	c.t.mtx.Lock()
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
