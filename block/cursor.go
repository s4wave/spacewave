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

// IsSubBlock indicates if the cursor is currently at a sub-block position.
func (c *Cursor) IsSubBlock() bool {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()
	return c.pos.isSubBlock
}

// Parent returns a new cursor pointing to the parent block.
// Note: the parent is completely dependent on the order the graph was traversed.
// TODO; It may be possible to have multiple parents.
func (c *Cursor) Parent() *Cursor {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	parent := c.pos.parent
	if parent == nil || parent.src == nil {
		return nil
	}
	src := parent.src
	return newCursor(c.t, src)
}

// SetRef sets a block reference to the handle at the cursor.
// Note: cannot use SetRef on / with sub-block cursors.
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
				c.t.blockGraph.RemoveEdge(c.pos.ID(), tgt.ID())
			}
		}
	}

	cursor.pos.parent = &refHandle{
		id:     refID,
		src:    c.pos,
		target: cursor.pos,
	}
	c.t.blockGraph.SetEdge(cursor.pos.parent)
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

	return c.followRef(refID, blkRef)
}

// followRef implements followRef assuming the mutex is locked
func (c *Cursor) followRef(refID uint32, blkRef *cid.BlockRef) *Cursor {
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref == nil {
		blkHandle := &handle{
			Node: c.t.blockGraph.NewNode(),
			ref:  blkRef,
		}
		ref = &refHandle{
			id:     refID,
			src:    c.pos,
			target: blkHandle,
		}
		blkHandle.parent = ref
		c.t.blockGraph.AddNode(blkHandle)
		c.t.blockGraph.SetEdge(ref)
		ref.target = blkHandle
		c.pos.refHandles[refID] = ref
	}

	return newCursor(c.t, ref.target)
}

// FollowSubBlock follows a sub-block reference, returning a cursor pointing to
// the same block but at a sub-block inside a field. The block is constructed or
// retrieved using the BlockWithSubBlocks interface.
//
// Note: there may already be a reference with the same ID, which would be returned.
// The cursor must have the block decoded or set with SetBlock.
// The cursor block blk must be a BlockWithSubBlocks.
// If these conditions are not met, returns nil
func (c *Cursor) FollowSubBlock(refID uint32) *Cursor {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	return c.followSubBlock(refID)
}

// followSubBlock implements followSubBlock
// The cursor must have the block decoded or set with SetBlock.
func (c *Cursor) followSubBlock(refID uint32) *Cursor {
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref == nil {
		cblk := c.pos.blk
		sbBlock, _ := cblk.(BlockWithSubBlocks)
		if sbBlock == nil {
			return nil
		}
		sbCtor := sbBlock.GetSubBlockCtor(refID)
		if sbCtor == nil {
			return nil
		}
		sbBlk := sbCtor()
		if sbBlk == nil {
			return nil
		}
		blkHandle := &handle{
			Node:       c.t.blockGraph.NewNode(),
			isSubBlock: true,
			blk:        sbBlk,
		}
		ref = &refHandle{
			id:     refID,
			src:    c.pos,
			target: blkHandle,
		}
		blkHandle.parent = ref
		c.t.blockGraph.AddNode(blkHandle)
		c.t.blockGraph.SetEdge(ref)
		ref.target = blkHandle
		c.pos.refHandles[refID] = ref
	}

	return newCursor(c.t, ref.target)
}

// ClearRef clears a block reference or sub-block.
// Noop if FollowRef has not been previously called with refid.
// Note: also refers to references from FollowSubBlock.
// Note: does not clear sub-blocks from the parent object.
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
		c.t.blockGraph.RemoveEdge(c.pos.ID(), tgt.ID())
	}
}

// ClearAllRefs clears all references.
func (c *Cursor) ClearAllRefs() {
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	if c.pos.refHandles == nil {
		return
	}
	for refID, r := range c.pos.refHandles {
		delete(c.pos.refHandles, refID)
		if tgt := r.target; tgt != nil {
			c.t.blockGraph.RemoveEdge(c.pos.ID(), tgt.ID())
		}
	}
}

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
// If a sub-block, will return the sub-block value or nil.
// Ctor is ignored if sub-block.
func (c *Cursor) Unmarshal(ctor func() Block) (Block, error) {
	if c == nil || c.t == nil {
		return nil, errors.New("nil cursor")
	}
	c.t.mtx.Lock()
	b := c.pos.blk
	isSubBlock := c.pos.isSubBlock
	c.t.mtx.Unlock()

	if isSubBlock {
		return b, nil
	}

	if b == nil && ctor != nil {
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
	c.pos.blkPreWrite = nil
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
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	posWithRefs, posWithRefsOk := c.pos.blk.(BlockWithRefs)
	if posWithRefsOk {
		blockRefs, err := posWithRefs.GetBlockRefs()
		if err != nil {
			return nil, err
		}
		if blockRefs != nil {
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
		}
	}
	posWithSubBlocks, posWithSubBlocksOk := c.pos.blk.(BlockWithSubBlocks)
	if posWithSubBlocksOk {
		subBlocks := posWithSubBlocks.GetSubBlocks()
		if subBlocks != nil {
			// load all non-nil sub blocks to ref handles
			for refID, blk := range subBlocks {
				if blk == nil {
					continue
				}
				if _, ok := c.pos.refHandles[refID]; ok {
					continue
				}
				m[refID] = c.followSubBlock(refID)
			}
		}
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
		ref := c.pos.parent
		for ref != nil {
			if ref.src.dirty {
				break
			}
			ref.src.dirty = true
			ref = ref.src.parent
		}
	}
}
