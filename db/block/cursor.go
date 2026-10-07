package block

import (
	"bytes"
	"context"
	"errors"
	"runtime/trace"

	"github.com/s4wave/spacewave/db/tx"
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
	store StoreOps
	// pos is the current block handle
	// if ephemeral, does not contain a block graph.
	pos *handle
}

// newCursor builds a new cursor.
func newCursor(t *Transaction, pos *handle, storeOverride StoreOps) *Cursor {
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

// IsDirty indicates if the position or any sub-positions were changed.
func (c *Cursor) IsDirty() bool {
	if c != nil && c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	if c == nil || c.pos == nil {
		return false
	}
	return c.pos.dirty
}

// GetTransaction returns the cursor's associated transaction, may be nil.
func (c *Cursor) GetTransaction() *Transaction {
	if c == nil {
		return nil
	}
	return c.t
}

// GetBlockStore returns the block store used for the transaction.
func (c *Cursor) GetBlockStore() (StoreOps, bool) {
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

// SetBlockStore sets the store to read from for this cursor and all sub-cursors.
// If nil, will use the default bucket attached to the block transaction.
func (c *Cursor) SetBlockStore(store StoreOps) {
	if c != nil {
		c.store = store
	}
}

// CloneBlock tries to clone the contained block.
//
// returns ErrUnexpectedType or ErrNotClonable if the block was not clonable.
func (c *Cursor) CloneBlock() (any, error) {
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	return CloneBlock(c.pos.blk)
}

// Detach clones the cursor position.
//
// If keepRefs is set, adds the new location as a parent of all previously
// referenced blocks.
//
// Note: does not copy/clone the Block object.
func (c *Cursor) Detach(keepRefs bool) *Cursor {
	// Ignore a missing cursor when detaching its position.
	if c == nil {
		return nil
	}

	// Protect the source position while cloning its graph links.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// clone the cursor
	nc := &Cursor{store: c.store, t: c.t}
	nc.pos = c.pos.Clone()
	nc.pos.blkPreWrite = nil
	nc.pos.isSubBlock = false

	// Register the detached position and retain its child links.
	if c.t != nil {
		nc.pos.Node = c.t.blockGraph.NewNode()
		c.t.blockGraph.AddNode(nc.pos)

		// Attach the detached position to each retained child block.
		if keepRefs {
			prevRefs := c.pos.refHandles
			for _, ref := range prevRefs {
				if ref.target == nil || ref.src != c.pos {
					continue
				}

				// Link the retained child to the detached cursor.
				tc := newCursor(nc.t, ref.target, nc.store)
				_ = tc.addParent(nc, ref.id)
			}
		}
	}

	return nc
}

// DetachTransaction creates a new ephemeral transaction rooted at the cursor.
// The detached cursor reads through the source transaction's staged writes, so
// it can load blocks the source staged but has not yet published.
func (c *Cursor) DetachTransaction() *Cursor {
	// Ignore a missing cursor when detaching its transaction.
	if c == nil {
		return nil
	}

	// Clone the cursor, keeping the source's view of staged blocks.
	store := c.store
	if store == nil && c.t != nil {
		if staged := c.t.GetStagedStore(); staged != nil {
			store = staged
		}
	}
	nc := &Cursor{store: store, t: c.t}
	nc.pos = c.pos.Clone()
	nc.pos.blkPreWrite = nil
	nc.pos.isSubBlock = false
	nc.t = c.t.cloneDetached(nc.pos, true)
	return nc
}

// DetachRecursive clones the cursor position and all referenced positions.
//
// Note: if !cloneBlocks, does not copy/clone the Block objects.
func (c *Cursor) DetachRecursive(detachTx, cloneBlocks, markDirty bool) *Cursor {
	// Ignore a missing cursor when detaching its block graph.
	if c == nil {
		return nil
	}

	// Protect the source graph while cloning its positions.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Clone the root position and select the detached transaction.
	oldTx := c.t
	nroot := c.pos.Clone()
	nroot.isSubBlock = false
	nc := &Cursor{store: c.store, pos: nroot, t: oldTx}
	if detachTx && c.t != nil {
		// detach the transaction
		nc.t = c.t.cloneDetached(nroot, !cloneBlocks)
	}

	// Copy the referenced positions into the detached graph.
	c.copyToRecursive(nc, cloneBlocks, markDirty)
	return nc
}

// Parents returns new cursors pointing to the parent blocks.
// Note: the parent list is completely dependent on the order the graph was traversed.
// Note: returns nil if the cursor is ephemeral (with Detach call).
func (c *Cursor) Parents() []*Cursor {
	// Ignore cursor positions without a transaction graph.
	if c == nil || c.t == nil || c.pos == nil {
		return nil
	}

	// Protect the parent edges while reading the cursor graph.
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	// Expose each parent position through a cursor.
	out := make([]*Cursor, len(c.pos.parents))
	for i, p := range c.pos.parents {
		out[i] = newCursor(c.t, p.src, c.store)
	}
	return out
}

// GetBlock returns the current loaded block at the position.
// May be nil if Fetch or Unmarshal or SetBlock have not been called.
// Returns isSubBlock.
func (c *Cursor) GetBlock() (any, bool) {
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
	// Ignore a missing cursor when replacing its block reference.
	if c == nil {
		return
	}

	// Protect the cursor position while replacing its reference.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Preserve the loaded block when its reference is unchanged.
	if ref != nil {
		if c.pos.ref != nil {
			if c.pos.ref.EqualsRef(ref) {
				return
			}
		}
	}

	// Replace the position reference and invalidate changed block data.
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
// The cursors must be from the same transaction.
// A clean stored block moved under c keeps its ref and is not encoded again.
// Note: this should only be used if refID and cursor are not sub-blocks.
func (c *Cursor) SetRef(refID uint32, cursor *Cursor) {
	// Clear the reference for a nil target and ignore invalid targets.
	if c == nil {
		return
	}
	if cursor == nil {
		c.ClearRef(refID)
		return
	}
	if cursor == c || cursor.pos == c.pos || cursor.t != c.t {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Detach the previous target of refID from c.
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	} else if r, ok := c.pos.refHandles[refID]; ok && r.target != nil {
		_ = newCursor(c.t, r.target, c.store).removeParent(c)
	}

	// A sub-block moved under a reference becomes a regular block.
	wasSubBlock := cursor.pos.isSubBlock
	if wasSubBlock {
		cursor.pos.isSubBlock = false
		cursor.pos.parents = nil
	}

	// Link the target to c, dropping the reference if linking fails.
	if cursor.addParent(c, refID) == nil {
		delete(c.pos.refHandles, refID)
		return
	}

	// A clean stored block keeps its ref; only its new parents change.
	unchanged := !cursor.pos.dirty || cursor.pos.moved
	moved := !wasSubBlock && unchanged && !cursor.pos.ref.GetEmpty()
	cursor.markDirty()
	cursor.pos.moved = moved
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
	// Ignore a missing cursor when following a block reference.
	if c == nil {
		return nil
	}

	// Reuse the traversed position for an existing reference.
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref != nil {
		return newCursor(c.t, ref.target, c.store)
	}

	// Create and attach the referenced position to the cursor graph.
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
	// Ignore a missing cursor when following a sub-block.
	if c == nil {
		return nil
	}

	// Reuse the traversed position for an existing sub-block.
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	ref := c.pos.refHandles[refID]
	if ref != nil {
		return newCursor(c.t, ref.target, c.store)
	}

	// Require a loaded block that exposes the requested sub-block.
	cblk := c.pos.blk
	sbBlock, _ := cblk.(BlockWithSubBlocks)
	if sbBlock == nil {
		return nil
	}

	// Resolve the constructor for the requested sub-block.
	sbCtor := sbBlock.GetSubBlockCtor(refID)
	if sbCtor == nil {
		return nil
	}

	// Load the sub-block value before creating its cursor.
	sbBlk := sbCtor(true)
	if sbBlk == nil || sbBlk.IsNil() {
		return nil
	}

	// Attach the loaded sub-block position to its parent cursor.
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

// SetAsSubBlock sets the cursor position as a sub-block of another block.
//
// Clears any existing parent references.
// Immediately calls ApplySubBlock on the parent block.
//
// May return ErrNotSubBlock or ErrUnexpectedType if the parent is not a block
// with sub-blocks.
func (c *Cursor) SetAsSubBlock(refID uint32, parent *Cursor) error {
	// Require distinct cursors in the same block transaction.
	if c == nil || parent == nil {
		return ErrNilCursor
	}
	if c.t != parent.t {
		return errors.New("cursors must share same block transaction")
	}
	if c == parent {
		return errors.New("cannot set cursor as sub-block of itself")
	}

	// Protect the cursor positions and require loaded parent and child blocks.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	if c.pos == nil || c.pos.blk == nil ||
		parent.pos == nil || parent.pos.blk == nil {
		return ErrNilBlock
	}

	// Require a parent that accepts the child as a sub-block and is not
	// shared with other readers.
	parentBlkWithSubBlocks, ok := parent.pos.blk.(BlockWithSubBlocks)
	if !ok {
		return ErrNotBlockWithSubBlocks
	}
	if c.t.sharesDecodedBlock(parent.pos.blk) {
		return tx.ErrNotWrite
	}
	subBlk, ok := c.pos.blk.(SubBlock)
	if !ok {
		return ErrNotSubBlock
	}

	// check if we need to clear the old ref
	prevRef := parent.pos.refHandles[refID]
	if prevRef != nil {
		if c.pos == prevRef.target && c.pos.isSubBlock {
			// no changes: already set
			return nil
		}
	}

	// remove all parents
	// sub-block cannot have multiple parents
	_ = c.removeParent(nil)

	// mark as sub-block and add parent
	c.pos.isSubBlock = true
	_ = c.addParent(parent, refID)

	// apply sub-block
	err := parentBlkWithSubBlocks.ApplySubBlock(refID, subBlk)
	if err != nil {
		return err
	}

	// we changed the parent, so mark it dirty
	parent.markDirty()
	return nil
}

// ClearRef removes the reference handle to the given ref ID.
// Noop if FollowRef has not been previously called with refID.
// Cursors that were generated by Follow() will be detached.
//
// Note: does not clear sub-blocks from the parent object.
// Note; does not clear the BlockRef from the parent object.
func (c *Cursor) ClearRef(refID uint32) {
	if c == nil {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}
	c.clearRef(refID)
}

// clearRef clears a reference removing the parent edge if necessary.
// expects caller to lock c.t.mtx
func (c *Cursor) clearRef(refID uint32) {
	// Find the traversed reference to detach from the cursor.
	if c.pos.refHandles == nil {
		return
	}
	r, ok := c.pos.refHandles[refID]
	if !ok {
		return
	}

	// clear parent relation
	if tgt := r.target; tgt != nil {
		tgtCursor := newCursor(c.t, tgt, c.store)
		tgtCursor.removeParent(c)
	}

	// clear ref handle
	delete(c.pos.refHandles, refID)
}

// ClearAllRefs clears all references.
// Cursors that were generated by Follow() will be detached.
//
// Note: does not clear sub-blocks from the parent object.
// Note; does not clear the BlockRef from the parent object.
func (c *Cursor) ClearAllRefs() {
	if c == nil {
		return
	}
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	c.clearRefHandles()
}

// Fetch fetches the block data into memory.
// Fetching is performed using a block lookup.
// Returns the transformed decoded version of the data.
// Returns data, dataIsSet, err.
// Returns nil, false, ErrBlockStoreUnavailable if the block store is unset.
// Returns nil, false, nil if the reference is empty.
// Returns nil, false, ErrNotFound if not found (block unavailable).
func (c *Cursor) Fetch(ctx context.Context) ([]byte, bool, error) {
	data, _, found, err := c.fetch(ctx)
	return data, found, err
}

// fetch loads the raw and unmarshaled block data at the cursor position.
func (c *Cursor) fetch(ctx context.Context) ([]byte, []byte, bool, error) {
	// Skip a nil cursor or an empty ref.
	if c == nil {
		return nil, nil, false, nil
	}
	if c.pos.ref.GetEmpty() {
		return nil, nil, false, nil
	}

	// Read the stored block, recording the read on the context.
	bkt := c.readStore(ctx)
	if bkt == nil {
		return nil, nil, false, ErrBlockStoreUnavailable
	}
	data, found, err := bkt.GetBlock(withoutReadOperationStore(ctx), c.pos.ref)
	if err == nil {
		recordReadCounter(ctx, found, len(data))
	}
	if err != nil || !found {
		if err == nil {
			err = ErrNotFound
		}
		return nil, nil, false, err
	}
	recordAccessLog(ctx, c.pos.ref, len(data))

	// Decode the stored data with the transformer, if any.
	storedData := data
	if xfrm := c.transformer(); xfrm != nil {
		if trace.IsEnabled() {
			_, task := trace.NewTask(ctx, "db/block/cursor/fetch/decode")
			storedData = bytes.Clone(data)
			data, err = xfrm.DecodeBlock(data)
			task.End()
		} else {
			storedData = bytes.Clone(data)
			data, err = xfrm.DecodeBlock(data)
		}
		if err != nil {
			return nil, nil, false, err
		}
	}
	return data, storedData, true, nil
}

// Unmarshal fetches and unmarshals the data to a block.
// If already unmarshaled, returns existing data.
// If a sub-block, the sub-block must implement Block.
// If a sub-block, will return the sub-block value or nil.
// ctor is ignored if the cursor is a sub-block.
// Returns nil, nil if ctor is nil and the block is nil.
// Returns nil, nil if the cursor is nil.
// Returns value from ctor() without calling Unmarshal if empty.
// Returns nil, block.ErrNotFound if not found.
func (c *Cursor) Unmarshal(ctx context.Context, ctor func() Block) (Block, error) {
	// Ignore a missing cursor when loading a block.
	if c == nil {
		return nil, nil
	}

	// Read the loaded block and sub-block status under the transaction lock.
	if c.t != nil {
		c.t.mtx.Lock()
	}
	blk := c.pos.blk
	isSubBlock := c.pos.isSubBlock
	if c.t != nil {
		c.t.mtx.Unlock()
	}

	// Require the loaded value to implement the block contract.
	b, err := CastToBlock(blk)
	if err != nil {
		return nil, err
	}

	// Reuse the loaded block when decoding is unnecessary.
	if b != nil || ctor == nil || isSubBlock {
		return b, nil
	}

	// note: ctor is called before fetch so the decoded cache can key by exact
	// block type before deciding whether storage must be touched.
	b = ctor()
	if b == nil {
		return nil, nil
	}

	// Identify the block cache entry using its concrete decoded type. A
	// read-only transaction shares immutable cached blocks without a clone.
	ctx = c.decodedBlockCacheContext(ctx)
	cacheKey, cacheable := decodedBlockCacheKeyFor(c.pos.ref, b, c.transformer())
	if !cacheable {
		RecordDecodedBlockUncacheable(ctx)
	}
	share := c.t.sharesDecodedBlock(b)

	// Refresh the block cache and reuse a cached decoded block.
	if cacheable {
		// Refresh the store state before looking up its decoded block.
		store := c.readStore(ctx)
		if freshener, ok := store.(DecodedBlockCacheFreshener); ok {
			if err := freshener.EnsureDecodedBlockCacheFresh(withoutReadOperationStore(ctx)); err != nil {
				return nil, err
			}
		}

		// Load the decoded block after refreshing its store state.
		cached, ok, err := lookupDecodedBlock(ctx, cacheKey, share)
		if err != nil {
			return nil, err
		}
		if ok {
			return c.setUnmarshaledBlock(cached)
		}
	}

	// Capture the cache store state before fetching the block.
	storeToken := decodedBlockCacheStoreToken{}
	if cacheable {
		storeToken = decodedBlockCacheStoreTokenFromContext(ctx, cacheKey.ref)
	}

	// returns nil, false, nil if reference was empty.
	// returns nil, false, ErrNotFound if reference was not found.
	dat, storedDat, datFound, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}

	// Decode the fetched bytes and retain cacheable block data.
	if datFound {
		// Decode the fetched bytes into the requested block type.
		recordDecodedBlockUnmarshal(ctx, len(dat))
		err := b.UnmarshalBlock(dat)
		if err != nil {
			return nil, err
		}

		// Cache the decoded block with its original stored bytes.
		if cacheable {
			if err := storeDecodedBlock(ctx, cacheKey, storeToken, c.pos.ref, b, storedDat, share); err != nil {
				return nil, err
			}
		}
	}

	return c.setUnmarshaledBlock(b)
}

// readStore returns the read-scoped store from the context, falling back
// to the cursor's block store.
func (c *Cursor) readStore(ctx context.Context) StoreOps {
	// Reuse the block store scoped to the current read operation.
	bkt := readOperationStore(ctx)
	if bkt != nil {
		return bkt
	}

	// Prefer the transaction view that includes staged blocks.
	if c != nil && c.store == nil && c.t != nil {
		if staged := c.t.GetStagedStore(); staged != nil {
			return staged
		}
	}

	// Resolve the cursor store when no read or staging scope applies.
	bkt, _ = c.GetBlockStore()
	return bkt
}

// decodedBlockCacheContext attaches the transaction's decoded block
// cache to the context when not already present.
func (c *Cursor) decodedBlockCacheContext(ctx context.Context) context.Context {
	// Preserve the context when a transaction cache is unnecessary.
	if decodedBlockCacheFromContext(ctx) != nil || c == nil || c.t == nil {
		return ctx
	}

	// Attach the transaction cache captured under its lock.
	c.t.mtx.Lock()
	cache := c.t.decodedBlocks
	c.t.mtx.Unlock()
	return WithDecodedBlockCache(ctx, cache)
}

// transformer returns the transaction's block transformer, or nil.
func (c *Cursor) transformer() Transformer {
	if c == nil || c.t == nil {
		return nil
	}
	return c.t.xfrm
}

// setUnmarshaledBlock caches the unmarshaled block on the cursor
// position, deduplicating concurrent unmarshal calls.
func (c *Cursor) setUnmarshaledBlock(b Block) (Block, error) {
	// Retain one decoded block per position under the transaction lock.
	var err error
	if c.t != nil {
		c.t.mtx.Lock()
	}
	if c.pos.blk != nil {
		// fixes race condition of two Unmarshal calls happen simultaneously.
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

// GetExistingRef checks if the reference has been traversed already.
// Returns nil if no handle exists for that ref.
func (c *Cursor) GetExistingRef(refID uint32) *Cursor {
	// Require a cursor position before resolving a traversed reference.
	if c == nil || c.pos == nil {
		return nil
	}

	// Protect the reference handles while resolving the target position.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Return the traversed reference target when it exists.
	ref := c.pos.refHandles[refID]
	if ref == nil {
		return nil
	}
	return newCursor(c.t, ref.target, c.store)
}

// SetPreWriteHook sets a hook for final transforms to the block.
//
// Note: this should not call any cursor functions that will be locked during
// the Write process.
//
// Also valid for sub-blocks.
func (c *Cursor) SetPreWriteHook(h func(b any) error) {
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
func (c *Cursor) SetBlock(b any, dirty bool) {
	// Ignore a missing cursor when replacing its loaded block.
	if c == nil {
		return
	}

	// Protect the cursor position while replacing its loaded block.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Replace the loaded block and invalidate its previous write hook.
	c.pos.blk = b
	c.pos.blkPreWrite = nil
	if b == nil {
		c.pos.ref = nil
	}

	// Propagate the changed block state to its ancestors.
	if dirty {
		c.markDirty()
	}
}

// GetAllRefs returns cursors to all references.
//
// If existingOnly, only returns references that have already been traversed.
// If !existingOnly uses GetSubBlocks and/or GetBlockRefs to list all references.
// If the position blk is empty, returns an empty map.
func (c *Cursor) GetAllRefs(existingOnly bool) (map[uint32]*Cursor, error) {
	// Prepare the reference result for the cursor position.
	m := map[uint32]*Cursor{}
	if c == nil {
		return m, nil
	}

	// Protect the cursor graph while resolving referenced blocks.
	if c.t != nil {
		c.t.mtx.Lock()
		defer c.t.mtx.Unlock()
	}

	// Require a loaded block before enumerating its references.
	if c.pos.blk == nil {
		return m, nil
	}

	// Collect the cursor targets that have already been traversed.
	for refID, refHandle := range c.pos.refHandles {
		if refHandle == nil || refHandle.target == nil {
			continue
		}
		m[refID] = newCursor(c.t, refHandle.target, c.store)
	}
	if existingOnly {
		return m, nil
	}

	// Resolve the loaded block references into cursor positions.
	if c.pos.refHandles == nil {
		c.pos.refHandles = make(map[uint32]*refHandle)
	}
	posWithRefs, posWithRefsOk := c.pos.blk.(BlockWithRefs)
	if posWithRefsOk {
		// Read the references declared by the loaded block.
		blockRefs, err := posWithRefs.GetBlockRefs()
		if err != nil {
			return nil, err
		}

		// load all block refs to ref handles
		for refID, bref := range blockRefs {
			// Ignore empty block references and targets already collected.
			if bref == nil || bref.GetEmpty() {
				continue
			}
			if _, ok := m[refID]; ok {
				continue
			}

			// Attach the unseen block reference to the cursor graph.
			m[refID] = c.followRef(refID, bref)
		}
	}

	// Resolve the loaded sub-blocks into cursor positions.
	posWithSubBlocks, posWithSubBlocksOk := c.pos.blk.(BlockWithSubBlocks)
	if posWithSubBlocksOk {
		// Read the sub-blocks declared by the loaded block.
		subBlocks := posWithSubBlocks.GetSubBlocks()

		// load all non-nil sub blocks to ref handles
		for refID, blk := range subBlocks {
			// Ignore absent sub-blocks and targets already collected.
			if blk == nil {
				continue
			}
			if _, ok := m[refID]; ok {
				continue
			}

			// Attach the unseen sub-block to the cursor graph.
			m[refID] = c.followSubBlock(refID)
		}
	}
	return m, nil
}

// markDirty marks the position and its ancestors dirty, clearing moved.
// Assumes c.t.mtx is locked.
func (c *Cursor) markDirty() {
	// Ephemeral cursors have nothing to write.
	if c == nil || c.t == nil {
		return
	}
	c.t.dirty = true
	if c.pos == nil {
		return
	}

	// Walk up until reaching positions already dirty with changed content.
	stk := []*handle{c.pos}
	for len(stk) != 0 {
		v := stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		if v.dirty && !v.moved {
			continue
		}
		v.dirty = true
		v.moved = false
		for _, ref := range v.parents {
			stk = append(stk, ref.src)
		}
	}
}

// addParent adds the given cursor as a parent of the location.
// assumes c.t.mtx is locked
func (c *Cursor) addParent(parent *Cursor, refID uint32) *refHandle {
	// Require distinct parent and child positions before linking them.
	if parent == nil || parent.pos == nil || c == nil || c.pos == nil {
		return nil
	}
	if parent.pos == c.pos || parent.pos.ID() == c.pos.ID() {
		// self edge: not allowed
		return nil
	}

	// Detach the previous target of the parent reference.
	removedEdges := make([]*refHandle, 0, 4)
	if parent.pos.refHandles == nil {
		parent.pos.refHandles = make(map[uint32]*refHandle)
	} else {
		oldEdge := parent.pos.refHandles[refID]
		if oldEdge != nil && oldEdge.target != nil {
			removedEdges = append(
				removedEdges,
				oldEdge.target.removeParent(oldEdge.src)...,
			)
		}
	}

	// Link the child position to its new parent.
	nedge := &refHandle{
		id:     refID,
		src:    parent.pos,
		target: c.pos,
	}
	removedEdges = append(removedEdges, c.pos.addParent(nedge)...)

	// Replace the displaced parent edges in the transaction graph.
	if c.t != nil && c.t.blockGraph != nil {
		for _, ref := range removedEdges {
			c.t.blockGraph.RemoveEdge(ref.src.ID(), ref.target.ID())
		}
		c.t.blockGraph.SetEdge(nedge)
	}

	// Retain the parent reference and propagate the child dirty state.
	parent.pos.refHandles[refID] = nedge
	if c.pos.dirty && !parent.pos.dirty {
		// mark parent dirty if necessary
		parent.markDirty()
	}
	return nedge
}

// removeParent removes the given cursor location as a parent.
// if parent == nil, removes all parents
//
// returns the old removed refhandles
func (c *Cursor) removeParent(parent *Cursor) []*refHandle {
	// Require a cursor position before removing parent edges.
	if c == nil || c.pos == nil {
		return nil
	}

	// Detach the requested parent edges from the cursor position.
	var removed []*refHandle
	if parent == nil || parent.pos == nil {
		// remove all parents
		removed = c.pos.parents
		c.pos.parents = nil
	} else {
		removed = c.pos.removeParent(parent.pos)
	}

	// Remove the detached edges from the transaction graph.
	if c.t != nil && c.t.blockGraph != nil {
		for _, ref := range removed {
			c.t.blockGraph.RemoveEdge(ref.src.ID(), ref.target.ID())
		}
	}

	// Remove parent reference handles that still target the detached edges.
	for _, ref := range removed {
		if ref.src.refHandles[ref.id] == ref {
			delete(ref.src.refHandles, ref.id)
		}
	}
	return removed
}

// clearRefHandles clears all ref handles.
// expects tx mtx to be locked
func (c *Cursor) clearRefHandles() {
	if c.pos.refHandles == nil {
		return
	}
	for refID, r := range c.pos.refHandles {
		if tgt := r.target; tgt != nil && c.t != nil {
			tgtCursor := newCursor(c.t, tgt, c.store)
			tgtCursor.removeParent(c)
		}
		delete(c.pos.refHandles, refID)
	}
}

// copyToRecursive detaches and/or copies to a new tx.
// caller must lock mtxs as needed
func (c *Cursor) copyToRecursive(targetCs *Cursor, cloneBlocks, markDirty bool) {
	// clone cursors down the tree
	remap := make(map[*handle]*handle, len(c.pos.refHandles)+1)

	// Reconnect copied positions to their copied parent handles.
	updParents := func(prevPos, nextPos *handle) {
		// Rebuild the copied position parent edges from the handle map.
		prevParents := prevPos.parents
		nextPos.parents = make([]*refHandle, 0, len(prevParents))
		for _, parent := range prevParents {
			// Ignore parents outside the copied portion of the graph.
			rstk, ok := remap[parent.src]
			if !ok {
				continue
			}

			// Reuse or create the edge connecting the copied positions.
			var nrh *refHandle
			if parent.src == rstk && parent.target == nextPos {
				nrh = parent
			} else {
				nrh = &refHandle{
					id:     parent.id,
					src:    rstk,
					target: nextPos,
				}
				if targetCs.t != nil {
					targetCs.t.blockGraph.SetEdge(nrh)
				}
			}

			// Retain the edge on both copied positions.
			nextPos.parents = append(nextPos.parents, nrh)
			if rstk.refHandles == nil {
				rstk.refHandles = make(map[uint32]*refHandle)
			}
			rstk.refHandles[nrh.id] = nrh

			// note: sub-block is not updated here. A read-only transaction
			// never writes, and its blocks may be shared with other readers.
			if !nextPos.isSubBlock && (targetCs.t == nil || !targetCs.t.readOnly.Load()) {
				if rblk, ok := rstk.blk.(BlockWithRefs); ok {
					// note: ignoring error here
					_ = rblk.ApplyBlockRef(nrh.id, nextPos.ref)
				}
			}
		}
	}

	// Traverse the source graph and map each position to its copy.
	prevRoot, nextRoot := c.pos, targetCs.pos
	stk := []*handle{prevRoot}
	for len(stk) != 0 {
		// Take the next source position from the traversal stack.
		nstk := stk[len(stk)-1]
		stk = stk[:len(stk)-1]

		// Reconnect parents when a shared source position is revisited.
		if _, ok := remap[nstk]; ok {
			// ensure all parents are updated
			updParents(nstk, nstk)
			continue
		}

		// Reuse the target root or clone the source child position.
		rstk := nstk
		if rstk == prevRoot {
			// reuse target root *handle
			rstk = nextRoot
		} else {
			rstk = nstk.Clone()
		}

		// Register the copied position in the target transaction graph.
		if rstk.Node == nil && targetCs.t != nil {
			rstk.Node = targetCs.t.blockGraph.NewNode()
			targetCs.t.blockGraph.AddNode(rstk.Node)
		}

		// update parents
		updParents(nstk, rstk)

		// clone block or clear if unable
		if cloneBlocks {
			// Recover a copied sub-block from its already copied parent.
			rstk.blk = nil
			if nstk.isSubBlock && len(rstk.parents) != 0 {
				// attempt to re-get the already cloned sub-block
				pref := rstk.parents[0]
				psub, ok := pref.src.blk.(BlockWithSubBlocks)
				if ok {
					ctor := psub.GetSubBlockCtor(pref.id)
					if ctor != nil {
						rstk.blk = ctor(true)
					}
				}
			}

			// Clone block data that could not be recovered from a parent.
			if !nstk.isSubBlock || (rstk.blk == nil && nstk.blk != nil) {
				// Clone block data that could not be recovered from a parent.
				// may return nil, ignore errors
				rstk.blk, _ = CloneBlock(nstk.blk)

				// update sub-blocks if necessary
				if nstk.isSubBlock && rstk.blk != nil {
					for _, pref := range rstk.parents {
						psub, ok := pref.src.blk.(BlockWithSubBlocks)
						if ok {
							subBlk, ok := rstk.blk.(SubBlock)
							if ok {
								_ = psub.ApplySubBlock(pref.id, subBlk)
							}
						}
					}
				}
			}
		}

		// Mark copied positions for writing when requested.
		if markDirty {
			rstk.dirty = true
		}

		// traverse child nodes
		for _, ref := range nstk.refHandles {
			stk = append(stk, ref.target)
		}

		// Retain the copied handle for subsequent parent reconstruction.
		remap[nstk] = rstk
	}
}
