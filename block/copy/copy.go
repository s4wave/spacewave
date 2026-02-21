// Package block_copy provides functions for copying block DAGs between stores.
package block_copy

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// CopyBlockDAG copies all blocks reachable from rootRef from src to dest.
// Skips blocks that already exist in dest (checked via GetBlockExists).
// rootCtor is the constructor for the root block type.
// For child blocks, uses BlockWithRefs.GetBlockRefCtor to get constructors.
// If a child block's constructor is nil, the block data is still copied
// but its children cannot be traversed (leaf copy).
func CopyBlockDAG(
	ctx context.Context,
	rootRef *block.BlockRef,
	rootCtor block.Ctor,
	src block.StoreOps,
	dest block.StoreOps,
) error {
	if rootRef.GetEmpty() {
		return nil
	}
	visited := make(map[string]bool)
	return copyBlock(ctx, rootRef, rootCtor, src, dest, visited)
}

// copyBlock copies a single block and recursively copies its children.
func copyBlock(
	ctx context.Context,
	ref *block.BlockRef,
	ctor block.Ctor,
	src block.StoreOps,
	dest block.StoreOps,
	visited map[string]bool,
) error {
	if ref.GetEmpty() {
		return nil
	}

	refStr := ref.MarshalString()
	if visited[refStr] {
		return nil
	}
	visited[refStr] = true

	// Check if already in dest.
	exists, err := dest.GetBlockExists(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "check block exists: %s", refStr)
	}
	if exists {
		return nil
	}

	// Read from source.
	data, found, err := src.GetBlock(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "get block: %s", refStr)
	}
	if !found {
		return errors.Wrapf(block.ErrNotFound, "block: %s", refStr)
	}

	// Write to dest.
	if _, _, err := dest.PutBlock(ctx, data, nil); err != nil {
		return errors.Wrapf(err, "put block: %s", refStr)
	}

	// Decode to find child refs (only if we have a constructor).
	if ctor == nil {
		return nil
	}
	blk := ctor()
	if err := blk.UnmarshalBlock(data); err != nil {
		return errors.Wrapf(err, "unmarshal block: %s", refStr)
	}

	// Follow child block refs.
	if err := followRefs(ctx, blk, src, dest, visited); err != nil {
		return err
	}

	// Check sub-blocks for refs too.
	if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
		for _, sub := range withSubBlocks.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			if err := followRefs(ctx, sub, src, dest, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

// followRefs checks if blk implements BlockWithRefs and recursively copies children.
func followRefs(
	ctx context.Context,
	blk any,
	src block.StoreOps,
	dest block.StoreOps,
	visited map[string]bool,
) error {
	withRefs, ok := blk.(block.BlockWithRefs)
	if !ok {
		return nil
	}
	refs, err := withRefs.GetBlockRefs()
	if err != nil {
		return errors.Wrap(err, "get block refs")
	}
	for id, childRef := range refs {
		childCtor := withRefs.GetBlockRefCtor(id)
		if err := copyBlock(ctx, childRef, childCtor, src, dest, visited); err != nil {
			return err
		}
	}
	return nil
}
