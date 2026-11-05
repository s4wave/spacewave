package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// GetBucketRoot returns the block the bucket's root name holds, or nil when
// the name is free.
func (v *Volume) GetBucketRoot(ctx context.Context, bucketID, name string) (*block.BlockRef, error) {
	var ref *block.BlockRef
	err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
		held, err := rg.GetOutgoingRefs(ctx, RootOwnerIRI(bucketID, name))
		if err != nil || len(held) == 0 {
			return err
		}
		ref, _ = block_gc.ParseBlockIRI(held[0])
		return nil
	})
	return ref, err
}

// ListBucketRoots calls fn with each root of the bucket whose name starts with
// prefix, in one snapshot.
func (v *Volume) ListBucketRoots(ctx context.Context, bucketID, prefix string, fn func(block.NamedRoot) error) error {
	// Read the roots in one snapshot, then call fn outside it.
	var roots []block.NamedRoot
	err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
		owners, err := rg.GetOutgoingRefs(ctx, block_gc.BucketIRI(bucketID))
		if err != nil {
			return err
		}
		for _, owner := range owners {
			b, name, ok := parseRootOwnerIRI(owner)
			if !ok || b != bucketID || len(name) < len(prefix) || name[:len(prefix)] != prefix {
				continue
			}
			held, err := rg.GetOutgoingRefs(ctx, owner)
			if err != nil {
				return err
			}
			if len(held) == 0 {
				continue
			}
			if ref, ok := block_gc.ParseBlockIRI(held[0]); ok {
				roots = append(roots, block.NamedRoot{Name: name, Ref: ref})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, root := range roots {
		if err := fn(root); err != nil {
			return err
		}
	}
	return nil
}

// GetRoot returns the block the Volume's own root name holds, as
// GetBucketRoot does for the bucket with the empty ID.
func (v *Volume) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return v.GetBucketRoot(ctx, "", name)
}

// ListRoots lists the Volume's own roots, as ListBucketRoots does for the
// bucket with the empty ID.
func (v *Volume) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return v.ListBucketRoots(ctx, "", prefix, fn)
}

// SetRoots moves the Volume's own roots, as SetBucketRoots does for the bucket
// with the empty ID. A Volume that cannot commit blocks and roots atomically
// holds no roots: it stores the entries and never frees by liveness.
func (v *Volume) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if !v.SupportsAtomicPublication() {
		return block.SetRootsUnheld(ctx, v, entries)
	}
	return v.SetBucketRoots(ctx, "", entries, roots)
}
