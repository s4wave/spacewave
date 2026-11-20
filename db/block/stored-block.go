package block

import "context"

// StoredBlock is a block's stored bytes with its outgoing refs.
type StoredBlock struct {
	// Data is the stored block data.
	Data []byte
	// Refs are the block's outgoing refs when RefsKnown is set.
	Refs []*BlockRef
	// RefsKnown is false when the source held the bytes without their refs,
	// so a leaf cannot be told from a block whose edges were lost.
	RefsKnown bool
	// Linked is set by the linked files' copy tier (see the filesegment
	// Store's GetCopyTier), which a copy chain reads after every Volume tier,
	// so a file on this Device is the block's only copy here, and a changed
	// file reads as missing. A DEX Store sets it, with no Data, for a linked
	// answer: a linked file is the block's only copy on the answering
	// Device (see dex_solicit.WithLinkedAnswers). The read path never sets
	// it, and a read-through fill never writes a Linked block.
	Linked bool
}

// GetBlockWithoutRefs implements GetStoredBlock for a store that keeps block
// bytes without their refs. Returns nil when the block is not found.
func GetBlockWithoutRefs(ctx context.Context, store StoreOps, ref *BlockRef) (*StoredBlock, error) {
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &StoredBlock{Data: data}, nil
}

// ReadStoredBlock reads a block from store, with its refs when withRefs is
// set. Returns nil when the block is not found.
func ReadStoredBlock(ctx context.Context, store StoreOps, ref *BlockRef, withRefs bool) (*StoredBlock, error) {
	if withRefs {
		return store.GetStoredBlock(ctx, ref)
	}
	return GetBlockWithoutRefs(ctx, store, ref)
}

// GetData returns the block data, or nil for a nil block.
func (b *StoredBlock) GetData() []byte {
	if b == nil {
		return nil
	}
	return b.Data
}

// GetRefs returns the block's outgoing refs, or nil for a nil block.
func (b *StoredBlock) GetRefs() []*BlockRef {
	if b == nil {
		return nil
	}
	return b.Refs
}

// GetRefsKnown reports whether the refs are known, false for a nil block.
func (b *StoredBlock) GetRefsKnown() bool {
	return b != nil && b.RefsKnown
}

// PutOpts returns options that store the block under ref with its refs.
// Refs are empty when they are unknown, so the caller decides whether a
// block without known refs may be written.
func (b *StoredBlock) PutOpts(ref *BlockRef) *PutOpts {
	return &PutOpts{ForceBlockRef: ref.Clone(), Refs: b.Refs, RefsKnown: b.RefsKnown}
}
