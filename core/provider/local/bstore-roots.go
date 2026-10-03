package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// SupportsRootRetention reports the underlying ownership capability.
func (b *BlockStore) SupportsRootRetention() bool { return block.SupportsRootRetention(b.store) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (b *BlockStore) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, b.store, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (b *BlockStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// A peer-backed root may not be local yet. Bring its bytes and refs into
	// the destination before acquiring a local pin; full graph retention
	// remains the copier's responsibility.
	found, err := b.store.GetBlockExists(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		stored, err := b.readOwner().GetStoredBlock(ctx, ref)
		if err != nil {
			return nil, err
		}
		if stored == nil {
			return nil, block.ErrNotFound
		}
		if _, _, err := b.store.PutBlock(ctx, stored.Data, stored.PutOpts(ref)); err != nil {
			return nil, err
		}
	}
	return block.PinRoot(ctx, b.store, ref)
}

// OpenStage wraps a stage opened on the local store. Reads keep the Session
// read path.
func (b *BlockStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// Open the stage on the local store.
	ops, release, err := block.OpenStage(ctx, b.store)
	if err != nil {
		return nil, nil, err
	}

	// Copy the store with its writes routed to the stage.
	staged := *b
	staged.store = block_store.NewStore(b.store.GetID(), ops)
	return &staged, release, nil
}

// ReleaseRoots forwards staging release to the underlying store.
func (b *BlockStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, b.store, refs)
}

// MarkRootsComplete forwards the durable World proof.
func (b *BlockStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, b.store, roots)
}

// RootComplete checks the underlying World proof.
func (b *BlockStore) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return block.RootComplete(ctx, b.store, ref)
}

// _ is a type assertion
var _ block.RootRetainer = (*BlockStore)(nil)
