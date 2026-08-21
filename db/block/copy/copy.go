// Package blockcopy provides functions for copying block DAGs between stores.
package blockcopy

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// CopyBlockDAG copies all blocks reachable from rootRef from src to dest.
// Missing blocks are written to dest. Existing destination blocks are still
// decoded and traversed when their constructors are known, so a partial
// destination DAG can be completed.
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
	// Start tracing and return immediately for an empty root.
	ctx, task := trace.NewTask(ctx, "hydra/block/copy/dag")
	defer task.End()

	if rootRef.GetEmpty() {
		trace.Log(ctx, "result", "empty-root")
		return nil
	}

	// Record the root and recurse through the destination graph.
	trace.Log(ctx, "root-ref", rootRef.MarshalString())
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
	// Skip empty or previously visited references before reading storage.
	if ref.GetEmpty() {
		return nil
	}

	// Start tracing this block copy and skip references already visited.
	refStr := ref.MarshalString()
	ctx, task := trace.NewTask(ctx, "hydra/block/copy/block")
	defer task.End()
	trace.Log(ctx, "block-ref", refStr)

	// Avoid traversing a reference more than once.
	if visited[refStr] {
		trace.Log(ctx, "result", "already-visited")
		return nil
	}
	visited[refStr] = true

	// Check whether the destination already contains this block.
	exists, err := dest.GetBlockExists(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "check block exists: %s", refStr)
	}

	var data []byte
	if exists {
		if ctor == nil {
			trace.Log(ctx, "result", "destination-leaf-exists")
			return nil
		}

		taskCtx, subtask := trace.NewTask(ctx, "hydra/block/copy/destination-get")
		var found bool
		data, found, err = dest.GetBlock(taskCtx, ref)
		subtask.End()
		if err != nil {
			trace.Log(ctx, "result", "destination-get-error")
			return errors.Wrapf(err, "get existing block: %s", refStr)
		}
		if !found {
			trace.Log(ctx, "result", "destination-missing")
			return errors.Wrapf(block.ErrNotFound, "existing block: %s", refStr)
		}
	}

	// Fetch missing blocks from the source and persist them in the destination.
	if !exists {
		// Read from source.
		taskCtx, subtask := trace.NewTask(ctx, "hydra/block/copy/source-get")
		var found bool
		data, found, err = src.GetBlock(taskCtx, ref)
		subtask.End()
		if err != nil {
			trace.Log(ctx, "result", "source-error")
			return errors.Wrapf(err, "get block: %s", refStr)
		}
		if !found {
			trace.Log(ctx, "result", "source-missing")
			return errors.Wrapf(block.ErrNotFound, "block: %s", refStr)
		}

		// Write to dest.
		taskCtx, subtask = trace.NewTask(ctx, "hydra/block/copy/destination-put")
		_, _, err = dest.PutBlock(taskCtx, data, nil)
		subtask.End()
		if err != nil {
			trace.Log(ctx, "result", "destination-error")
			return errors.Wrapf(err, "put block: %s", refStr)
		}
	}

	// Decode copied data and recurse through known child references.
	if ctor == nil {
		trace.Log(ctx, "result", "leaf-copied")
		return nil
	}
	blk := ctor()
	if err := blk.UnmarshalBlock(data); err != nil {
		trace.Log(ctx, "result", "unmarshal-error")
		return errors.Wrapf(err, "unmarshal block: %s", refStr)
	}

	if err := followBlockGraph(ctx, blk, src, dest, visited); err != nil {
		return err
	}

	if exists {
		trace.Log(ctx, "result", "destination-exists-traversed")
		return nil
	}
	trace.Log(ctx, "result", "copied")
	return nil
}

// followBlockGraph follows refs on a block or sub-block and then descends
// through nested sub-blocks.
func followBlockGraph(
	ctx context.Context,
	blk any,
	src block.StoreOps,
	dest block.StoreOps,
	visited map[string]bool,
) error {
	// Copy direct block references before descending into nested sub-blocks.
	withRefs, ok := blk.(block.BlockWithRefs)
	if ok {
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
	}

	// Recurse through each non-empty nested sub-block.
	if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
		for _, sub := range withSubBlocks.GetSubBlocks() {
			if sub == nil || sub.IsNil() {
				continue
			}
			if err := followBlockGraph(ctx, sub, src, dest, visited); err != nil {
				return err
			}
		}
	}
	return nil
}
