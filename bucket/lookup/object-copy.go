package bucket_lookup

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// CopyObjectToBucket copies an object from srcCursor to destCursor.
//
// rootCtor must construct the block located at srcCursor.
//
// The concurrency limit controls how many concurrent read/writes can be called.
// If maxConcurrency <= 0, has no limit on concurrent read/writes.
//
// copies from srcCursor to destCursor using the transform from srcCursor
// returns the updated object ref in the destination cursor.
// sets the bucket id and transform config directly in the returned ref.
//
// skipSubtreeExists skips a block ref tree if the block already existed in the
// target storage. this assumes that a block existing in storage implies that
// all blocks it references have also already been stored.
//
// cb is an optional callback to call with each block before copying.
// if cb is nil and a block is not found, returns block.ErrNotFound
func CopyObjectToBucket(
	ctx context.Context,
	destCursor, srcCursor *Cursor,
	rootCtor block.Ctor,
	maxConcurrency int,
	skipSubtreeExists bool,
	cb WalkObjectBlocksCb,
) (*bucket.ObjectRef, error) {
	// transform the destination object ref (for returning)
	srcRef := srcCursor.GetRef()
	destinationRef := srcRef.Clone()
	destinationRef.BucketId = destCursor.GetOpArgs().GetBucketId()
	destinationRef.TransformConf = srcCursor.GetTransformConf().Clone()
	destinationRef.TransformConfRef = nil

	// if the cursors are located in the same bucket and volume, do nothing.
	if srcCursor.GetOpArgs().EqualVT(destCursor.GetOpArgs()) {
		return destinationRef, nil
	}

	writeCursor, err := destCursor.FollowRef(ctx, destinationRef)
	if err != nil {
		if err == context.Canceled {
			return nil, err
		}
		return nil, errors.Wrap(err, "construct write cursor")
	}
	defer writeCursor.Release()

	readBkt := srcCursor.GetBucket()
	readXfrm := srcCursor.GetTransformer()
	writeBkt := writeCursor.GetBucket()

	// Ensure we do not process duplicate blocks by tracking which blocks were seen.
	// use a sync.Map since this is the exact situation it is meant for
	// key: string (BlockRef)
	// value: bool (seen)
	var seenBlocks sync.Map

	// To copy the object fully, we have to traverse the block graph.
	// We do this by recursively following the block refs.
	// Note that GetBlockRefCtor must be implemented for this to work properly.
	// TODO: handle garbage collection (set parent in PutOpts)
	if err := WalkObjectBlocks(
		ctx,
		NewWalkObjectBlocksWithRef(srcRef.GetRootRef(), rootCtor),
		func(ent *WalkObjectBlocksEntry) (cntu bool, err error) {
			// call the callback if set
			if cb != nil {
				cntu, err = cb(ent)
			} else {
				// Note: we give the callback the chance to ignore the err above.
				err = ent.Err
				if err == nil && !ent.Found && !ent.IsSubBlock && !ent.Ref.GetEmpty() {
					err = errors.Wrap(block.ErrNotFound, ent.Ref.MarshalString())
				}
				cntu = err == nil
			}

			if err != nil || ent.IsSubBlock || !ent.Found || ent.Ref.GetEmpty() || len(ent.Data) == 0 {
				// skip this block since it is not found or a sub-block or empty
				return cntu, err
			}

			// skip copying if we already saw this block
			refStr := ent.Ref.MarshalString()
			_, seen := seenBlocks.LoadOrStore(refStr, true)
			if seen {
				return
			}

			// copy the block
			// note: most implementations check Exists() inside PutBlock().
			var writeRef *block.BlockRef
			var writeExisted bool
			writeRef, writeExisted, err = writeBkt.PutBlock(ctx, ent.Data, &block.PutOpts{
				HashType:      ent.Ref.GetHash().GetHashType(),
				ForceBlockRef: ent.Ref,
			})
			if err == nil && !writeRef.EqualsRef(ent.Ref) {
				err = errors.Errorf("wrote to different ref %s", writeRef.MarshalString())
			}
			if err != nil && err != context.Canceled {
				err = errors.Wrapf(err, "write ref %s", ent.Ref.MarshalString())
			}

			if skipSubtreeExists && writeExisted && err == nil {
				// skip sub-tree
				return false, nil
			}

			return err == nil, err
		},
		readBkt, readXfrm,
		maxConcurrency,
		false,
	); err != nil {
		return nil, err
	}

	return destinationRef, nil
}
