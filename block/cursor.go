package block

import (
	"errors"
)

// Cursor tracks traversal of a block reference DAG structure with an associated
// Transaction. Manages interacting with block handles, the transaction cache,
// the decoder and marshaller, and the transformers.
type Cursor struct {
	// t is the transaction
	// if nil: cursor is ephemeral (no associated block graph)
	t *Transaction
	// store is the block store to read from
	// if nil, use the store from transaction.
	store Store
	// pos is the current block handle
	// if ephemeral, does not contain a block graph.
	pos *handle
}

// newCursor builds a new cursor.
func newCursor(t *Transaction, pos *handle, storeOverride Store) *Cursor {
	return &Cursor{t: t, pos: pos, store: storeOverride}
}

// IsSubBlock indicates if the cursor is currently at a sub-block position.
func (c *Cursor) IsSubBlock() bool {
	if c == nil {
		return false
	}

	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	return c.pos.isSubBlock
}

// SetBlockStore sets the store to read from for this cursor and all sub-cursors.
// If nil, will use the default bucket attached to the block transaction.
func (c *Cursor) SetBlockStore(store Store) {
	if c != nil {
		c.store = store
	}
}

// GetTransaction returns the cursor's associated transaction, may be nil.
func (c *Cursor) GetTransaction() *Transaction {
	if c == nil {
		return nil
	}
	return c.t
}

// GetBlockStore returns the block store used for the transaction.
func (c *Cursor) GetBlockStore() (Store, bool) {
	if c != nil {
		if c.store != nil {
			return c.store, true
		}
		if c.t != nil {
			return c.t.store, false
		}
	}
	return nil, false
}

// Detach clones the cursor position and clears the parent and child refs.
// Note: does not copy the Block object internally.
//
// If ephemeral is set, creates a new block transaction rooted at bcs.
func (c *Cursor) Detach(ephemeral bool) *Cursor {
	if c == nil {
		return nil
	}

	// clone the cursor
	nc := &Cursor{store: c.store, t: c.t}
	nc.pos = c.pos.Clone()
	nc.pos.blkPreWrite = nil
	nc.pos.isSubBlock = false

	if ephemeral {
		nc.t = c.t.cloneDetached(nc.pos)
	} else if c.t != nil {
		nc.pos.Node = c.t.blockGraph.NewNode()
		c.t.blockGraph.AddNode(nc.pos)
		if nc.store == nil && c.t != nil {
			nc.store = c.t.store
		}
	}

	return nc
}

// Parents returns new cursors pointing to the parent blocks.
// Note: the parent list is completely dependent on the order the graph was traversed.
// Note: returns nil if the cursor is ephemeral (with Detach call).
func (c *Cursor) Parents() []*Cursor {
	if c == nil || c.t == nil || c.pos == nil {
		return nil
	}

	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	out := make([]*Cursor, len(c.pos.parents))
	for i, p := range c.pos.parents {
		out[i] = newCursor(c.t, p.src, c.store)
	}
	return out
}

// GetBlock returns the current loaded block at the position.
// May be nil if Fetch or Unmarshal or SetBlock have not been called.
// Returns isSubBlock.
func (c *Cursor) GetBlock() (interface{}, bool) {
	if c == nil {
		return nil, false
	}

	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	return c.pos.blk, c.pos.isSubBlock
}

// SetRefAtCursor sets the reference at the cursor location.
// If ref is not equal to the existing ref, and clearBlock is set, blk is set to nil.
func (c *Cursor) SetRefAtCursor(ref *BlockRef, clearBlock bool) {
	if c == nil {
		return
	}

	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	if ref != nil {
		if c.pos.ref != nil {
			if c.pos.ref.EqualsRef(ref) {
				return
			}
		}
	}

	dirty := c.pos.ref != ref
	c.pos.ref = ref
	if dirty {
		if clearBlock {
			c.pos.blk = nil
			c.pos.blkPreWrite = nil
		}
		c.markDirty()
	}
}

// SetRef sets a block reference to the handle at the cursor.
// Adds c to the list of parents for cursor.
// Note: this should only be used if refID and cursor are not sub-blocks.
func (c *Cursor) SetRef(refID uint32, cursor *Cursor) {
	if c == nil {
		return
	}
	if cursor == nil {
		c.ClearRef(refID)
		return
	}
	if cursor == c || cursor.pos == c.pos {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	} else {
		// clear old destination parent relation
		if r, ok := c.pos.refHandles[refID]; ok {
			// value is changed below, clear old parent ref
			if tgt := r.target; tgt != nil {
				tgtCs := newCursor(c.t, tgt, c.store)
				_ = tgtCs.removeParent(c)
			}
		}
	}

	// add parent relation
	np := cursor.addParent(c, refID)
	if np != nil {
		cursor.markDirty()
	} else {
		delete(c.pos.refHandles, refID)
	}
}

// MarkDirty marks the cursor location dirty, so that it will be re-written.
//
// Note: if cursor is ephemeral (no transaction) this is no-op.
func (c *Cursor) MarkDirty() {
	if c == nil || c.t == nil {
		return
	}

	c.t.mtx.Lock()
	c.markDirty()
	c.t.mtx.Unlock()
}

// FollowRef follows a block reference, returning a cursor pointing to the next
// block and enqueuing the block for fetching. Does not wait for the block to be
// fetched to return. If the reference is empty, will create a new block.
func (c *Cursor) FollowRef(
	refID uint32,
	blkRef *BlockRef,
) *Cursor {
	if c == nil {
		return nil
	}

	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	return c.followRef(refID, blkRef)
}

// followRef implements followRef assuming the mutex is locked
func (c *Cursor) followRef(refID uint32, blkRef *BlockRef) *Cursor {
	if c == nil {
		return nil
	}

	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref != nil {
		return newCursor(c.t, ref.target, c.store)
	}
	blkHandle := &handle{ref: blkRef}
	if c.t != nil {
		blkHandle.Node = c.t.blockGraph.NewNode()
	}
	ref = &refHandle{
		id:     refID,
		src:    c.pos,
		target: blkHandle,
	}
	outCursor := newCursor(c.t, blkHandle, c.store)
	_ = outCursor.addParent(c, refID)
	return outCursor
}

// FollowSubBlock follows a sub-block reference, returning a cursor pointing to
// the same block but at a sub-block inside a field. The block is constructed or
// retrieved using the BlockWithSubBlocks interface.
//
// Once FollowSubBlock has been called, the field will be overwritten if dirty.
// If ClearRef is called on the parent then this relation is removed.
//
// Note: there may already be a reference with the same ID, which would be returned.
// The cursor must have the block decoded or set with SetBlock.
// The cursor block blk must be a BlockWithSubBlocks.
// If these conditions are not met, returns nil
func (c *Cursor) FollowSubBlock(refID uint32) *Cursor {
	if c == nil {
		return nil
	}

	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	return c.followSubBlock(refID)
}

// followSubBlock implements followSubBlock
// The cursor must have the block decoded or set with SetBlock.
func (c *Cursor) followSubBlock(refID uint32) *Cursor {
	if c == nil {
		return nil
	}
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref != nil {
		return newCursor(c.t, ref.target, c.store)
	}

	cblk := c.pos.blk
	sbBlock, _ := cblk.(BlockWithSubBlocks)
	if sbBlock == nil {
		return nil
	}
	sbCtor := sbBlock.GetSubBlockCtor(refID)
	if sbCtor == nil {
		return nil
	}
	sbBlk := sbCtor(true)
	if sbBlk == nil {
		return nil
	}
	blkHandle := &handle{
		isSubBlock: true,
		blk:        sbBlk,
	}
	if c.t != nil {
		blkHandle.Node = c.t.blockGraph.NewNode()
	}
	outCursor := newCursor(c.t, blkHandle, c.store)
	_ = outCursor.addParent(c, refID)
	return outCursor
}

// SetAsSubBlock sets the position the cursor points to as a sub-block of
// another block. Clears any existing parent references. Internally, immediately
// calls ApplySubBlock on the parent block.
//
// May return ErrNotSubBlock or ErrUnexpectedType if the parent is not a block
// with sub-blocks.
func (c *Cursor) SetAsSubBlock(refID uint32, parent *Cursor) error {
	if c == nil || parent == nil {
		return ErrNilCursor
	}
	if c.t != parent.t {
		return errors.New("cursors must share same block transaction")
	}
	if c == parent {
		return errors.New("cannot set cursor as sub-block of itself")
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	if c.pos == nil || c.pos.blk == nil ||
		parent.pos == nil || parent.pos.blk == nil {
		return ErrNilBlock
	}
	parentBlkWithSubBlocks, ok := parent.pos.blk.(BlockWithSubBlocks)
	if !ok {
		return ErrNotBlockWithSubBlocks
	}
	err := parentBlkWithSubBlocks.ApplySubBlock(refID, c.pos.blk)
	if err != nil {
		return err
	}
	_ = c.removeParent(nil) // remove all parents
	c.pos.isSubBlock = true
	_ = c.addParent(parent, refID)
	return nil
}

// ClearRef clears a block reference or sub-block.
// Noop if FollowRef has not been previously called with refid.
// Note: also refers to references from FollowSubBlock.
// Note: does not clear sub-blocks from the parent object.
func (c *Cursor) ClearRef(refID uint32) {
	if c == nil {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	if c.pos.refHandles == nil {
		return
	}
	r, ok := c.pos.refHandles[refID]
	if !ok {
		return
	}
	delete(c.pos.refHandles, refID)
	// clear parent relation
	if tgt := r.target; tgt != nil && c.t != nil {
		tgtCursor := newCursor(c.t, tgt, c.store)
		tgtCursor.removeParent(c)
	}
}

// ClearAllRefs clears all references.
// Removes the cursor as the parent for all referenced blocks.
func (c *Cursor) ClearAllRefs() {
	if c == nil {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	if c.pos.refHandles == nil {
		return
	}
	for refID, r := range c.pos.refHandles {
		delete(c.pos.refHandles, refID)
		if tgt := r.target; tgt != nil && c.t != nil {
			tgtCursor := newCursor(c.t, tgt, c.store)
			tgtCursor.removeParent(c)
		}
	}
}

// Fetch fetches the block data into memory.
// Fetching is performed using a block lookup.
func (c *Cursor) Fetch() ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	if c.pos.ref.GetEmpty() {
		return nil, false, nil
	}

	bkt, _ := c.GetBlockStore()
	if bkt == nil {
		return nil, false, ErrBucketUnavailable
	}
	return bkt.GetBlock(c.pos.ref)
}

// Unmarshal fetches and unmarshals the data to a block.
// If already unmarshaled, returns existing data.
// Returns found, error
// If a sub-block, will return the sub-block value or nil.
// Ctor is ignored if sub-block.
// If a sub-block, the sub-block must implement Block.
func (c *Cursor) Unmarshal(ctor func() Block) (Block, error) {
	if c == nil {
		return nil, errors.New("nil cursor")
	}
	if c.t != nil {
		c.t.mtx.Lock()
	}
	blk := c.pos.blk
	isSubBlock := c.pos.isSubBlock
	if c.t != nil {
		c.t.mtx.Unlock()
	}

	b, err := CastToBlock(blk)
	if err != nil {
		return nil, err
	}

	if b != nil || ctor == nil || isSubBlock {
		return b, nil
	}

	b = ctor()
	if b == nil {
		return nil, errors.New("block constructor returned nil")
	}

	dat, ok, err := c.Fetch()
	if err != nil || !ok {
		return nil, err
	}

	if err := b.UnmarshalBlock(dat); err != nil {
		return nil, err
	}

	if c.t != nil {
		c.t.mtx.Lock()
	}
	if c.pos.blk != nil {
		b, err = CastToBlock(c.pos.blk)
	} else {
		c.pos.blk = b
	}
	if c.t != nil {
		c.t.mtx.Unlock()
	}
	return b, err
}

// GetRef returns the current cursor reference.
func (c *Cursor) GetRef() *BlockRef {
	if c == nil || c.pos == nil {
		return nil
	}
	return c.pos.ref
}

// SetPreWriteHook sets a hook for final transforms to the block.
//
// Note: this should not call any cursor functions that will be locked during
// the Write process.
//
// Also valid for sub-blocks.
func (c *Cursor) SetPreWriteHook(h func(b interface{}) error) {
	if c != nil {
		c.pos.blkPreWrite = h
	}
}

// SetBlock sets a block at the location, and marks the block as dirty.
// If the location is a Block, b should implement Block interface.
// If it is a SubBlock, b should implement the SubBlock interface.
// If dirty is set, sets the block as dirty.
//
// Clears BlockPreWrite.
func (c *Cursor) SetBlock(b interface{}, dirty bool) {
	if c == nil {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	c.pos.blk = b
	c.pos.blkPreWrite = nil
	if b == nil {
		c.pos.ref = nil
	}
	if dirty {
		c.markDirty()
	}
}

// GetBlockRefs returns cursors to all pending / not pending references.
// If the position blk is empty, returns an empty map.
func (c *Cursor) GetAllRefs() (map[uint32]*Cursor, error) {
	m := map[uint32]*Cursor{}
	if c == nil {
		return m, nil
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

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
		m[refID] = newCursor(c.t, refHandle.target, c.store)
	}
	return m, nil
}

// markDirty assumes c.t.mtx is locked
func (c *Cursor) markDirty() {
	if c == nil || c.t == nil {
		return
	}
	c.t.dirty = true
	if c.pos != nil {
		stk := []*handle{c.pos}
		for len(stk) != 0 {
			v := stk[len(stk)-1]
			stk = stk[:len(stk)-1]
			if !v.dirty {
				v.dirty = true
				for _, ref := range v.parents {
					stk = append(stk, ref.src)
				}
			}
		}
	}
}

// addParent adds the given cursor as a parent of the location.
func (c *Cursor) addParent(parent *Cursor, refID uint32) *refHandle {
	if parent == nil || parent.pos == nil || c == nil || c.pos == nil {
		return nil
	}
	if parent.pos == c.pos || parent.pos.Node.ID() == c.pos.Node.ID() {
		// self edge: not allowed
		return nil
	}
	nedge := &refHandle{
		id:     refID,
		src:    parent.pos,
		target: c.pos,
	}
	removed := c.pos.addParent(nedge)
	if c.t != nil && c.t.blockGraph != nil {
		// if the edge already existed, remove it first
		for _, ref := range removed {
			c.t.blockGraph.RemoveEdge(ref.src.ID(), ref.target.ID())
		}
		c.t.blockGraph.SetEdge(nedge)
	}
	if parent.pos.refHandles == nil {
		parent.pos.refHandles = make(map[uint32]*refHandle)
	}
	parent.pos.refHandles[refID] = nedge
	return nedge
}

// removeParent removes the given cursor location as a parent.
// if parent == nil, removes all parents
//
// returns the old removed refhandles
func (c *Cursor) removeParent(parent *Cursor) []*refHandle {
	if c == nil || c.pos == nil {
		return nil
	}
	var removed []*refHandle
	if parent == nil || parent.pos == nil {
		// remove all parents
		removed = c.pos.parents
		c.pos.parents = nil
	} else {
		removed = c.pos.removeParent(parent.pos)
	}
	if c.t != nil && c.t.blockGraph != nil {
		for _, ref := range removed {
			c.t.blockGraph.RemoveEdge(ref.src.ID(), ref.target.ID())
		}
	}
	return removed
}
