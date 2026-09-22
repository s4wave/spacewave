package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// SupportsRootRetention reports the underlying ownership capability.
func (b *BlockStore) SupportsRootRetention() bool { return block.SupportsRootRetention(b.store) }

// SetRetainedRoot forwards durable root ownership to the underlying store.
func (b *BlockStore) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	return block.SetRetainedRoot(ctx, b.store, name, ref)
}

// PinRoot forwards a reader pin to the underlying store.
func (b *BlockStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// A peer-backed root may not be local yet. Bring its bytes into the
	// destination before acquiring a local pin; full graph retention remains
	// the copier's responsibility.
	found, err := b.store.GetBlockExists(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		data, found, err := b.readOwner().GetBlock(ctx, ref)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, block.ErrNotFound
		}
		if _, _, err := b.store.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
			return nil, err
		}
	}
	return block.PinRoot(ctx, b.store, ref)
}

// MarkRootsComplete forwards the durable World proof.
func (b *BlockStore) MarkRootsComplete(ctx context.Context, proofs []block.RootProof) error {
	return block.MarkRootsComplete(ctx, b.store, proofs)
}

// RootComplete checks the underlying World proof.
func (b *BlockStore) RootComplete(ctx context.Context, ref *block.BlockRef, domain ...string) (bool, error) {
	return block.RootComplete(ctx, b.store, ref, domain...)
}
