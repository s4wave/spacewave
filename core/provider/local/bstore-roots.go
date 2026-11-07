package provider_local

import (
	"context"
	fmt "fmt"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// HoldsRoots reports whether the underlying store holds roots.
func (b *BlockStore) HoldsRoots() bool { return block.HoldsRoots(b.store) }

// GetRoot reads the root from the underlying store.
func (b *BlockStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return b.store.GetRoot(ctx, name)
}

// ListRoots lists the roots of the underlying store.
func (b *BlockStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return b.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the underlying store. A root moving onto a
// block the local store lacks, such as a follower's replay base on a World
// another device wrote, brings the block's bytes and refs from the read path
// into the same change, so the local store never holds a root on a block it
// has not stored. A block the read path holds without its refs refuses the
// change.
func (b *BlockStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Index the blocks the change stores itself.
	batch := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		batch[e.Ref.MarshalString()] = struct{}{}
	}

	// Bring each other root block the local store lacks.
	for _, root := range roots {
		if root.Ref.GetEmpty() {
			continue
		}
		if _, ok := batch[root.Ref.MarshalString()]; ok {
			continue
		}
		entry, err := b.localRootEntry(ctx, root.Ref)
		if err != nil {
			return err
		}
		if entry != nil {
			entries = append(entries, entry)
			batch[root.Ref.MarshalString()] = struct{}{}
		}
	}

	// Push the committed blocks beside the local root change when configured.
	if b.push != nil {
		return b.push.SetRoots(ctx, entries, roots, b.store.SetRoots)
	}
	return b.store.SetRoots(ctx, entries, roots)
}

// localRootEntry returns the entry that stores ref locally with its refs, or
// nil when the local store holds it.
func (b *BlockStore) localRootEntry(ctx context.Context, ref *block.BlockRef) (*block.PutBatchEntry, error) {
	// Skip a block the local store holds.
	found, err := b.store.GetBlockExists(ctx, ref)
	if err != nil || found {
		return nil, err
	}

	// Read it with its refs from the read path.
	stored, err := b.readOwner().GetStoredBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("local provider root: %w", block.NotFoundError(ref))
	}
	if !stored.RefsKnown {
		return nil, errors.Wrap(block.ErrRefsUnknown, ref.MarshalString())
	}
	return &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true}, nil
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
			return nil, fmt.Errorf("local provider root pin: %w", block.NotFoundError(ref))
		}
		if !stored.RefsKnown {
			return nil, errors.Wrap(block.ErrRefsUnknown, ref.MarshalString())
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

// RootsComplete checks the underlying World proofs.
func (b *BlockStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, b.store, refs)
}

// HoldCompleteRoots holds completion proofs under the underlying stage.
func (b *BlockStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, b.store, refs)
}

// _ is a type assertion
var (
	_ block.LeasedRoots = (*BlockStore)(nil)
	_ block.RootProofs  = (*BlockStore)(nil)
)
