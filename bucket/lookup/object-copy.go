package bucket_lookup

import (
	"context"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

// CopyObjectToBucket copies an object from srcCursor to destCursor.
//
// rootCtor must construct the block located at srcCursor.
//
// copies from srcCursor to destCursor using the transform from srcCursor
// returns the updated object ref in the destination cursor.
// sets the bucket id and transform config directly in the returned ref.
func CopyObjectToBucket(ctx context.Context, destCursor, srcCursor *Cursor, rootCtor block.Ctor) (*bucket.ObjectRef, error) {
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

	// To copy the object fully, we have to traverse the block graph.
	// We do this by recursively following the block refs.
	// Note that GetBlockRefCtor must be implemented for this to work properly.
	// TODO: handle garbage collection (set parent in PutOpts)
	type stackElem struct {
		ref  *block.BlockRef
		ctor block.Ctor
		blk  interface{}
	}
	_, bcs := srcCursor.BuildTransactionAtRef(nil, destinationRef.GetRootRef())
	stack := []stackElem{{ref: bcs.GetRef(), ctor: rootCtor}}
	for len(stack) != 0 {
		elem := stack[len(stack)-1]
		// stack[len(stack)-1] = nil
		stack = stack[:len(stack)-1]

		blk := elem.blk
		if elem.ref != nil {
			// returns nil, false, nil if reference was empty.
			// returns nil, false, ErrNotFound if reference was not found.
			dat, found, err := readBkt.GetBlock(elem.ref)
			if err == nil && !found {
				err = block.ErrNotFound
			}
			if err != nil {
				if err == context.Canceled {
					return nil, err
				}
				return nil, errors.Wrapf(err, "fetch ref %s", elem.ref.MarshalString())
			}

			// NOTE: don't use GetBlockExists here as this goes to the "read" bucket handle.
			// if exists, existsErr := writeBkt.GetBlockExists(elem.ref); !exists || existsErr != nil {
			{
				// copy the block
				writeRef, _, err := writeBkt.PutBlock(dat, &block.PutOpts{
					HashType:      elem.ref.GetHash().GetHashType(),
					ForceBlockRef: elem.ref,
				})
				if err == nil && !writeRef.EqualsRef(elem.ref) {
					err = errors.Errorf("wrote to different ref %s", writeRef.MarshalString())
				}
				if err != nil {
					return nil, errors.Wrapf(err, "write ref %s", elem.ref.MarshalString())
				}
			}

			if elem.ctor == nil {
				// le.Warnf("ref %s: skipped block without ctor", elem.ref.MarshalString())
				continue
			}

			// construct the block
			decodeBlk := elem.ctor()

			// skip block if it has no sub-blocks or refs
			switch decodeBlk.(type) {
			case block.BlockWithRefs:
			case block.BlockWithSubBlocks:
			default:
				continue
			}

			// transform data
			dat, err = readXfrm.DecodeBlock(dat)
			if err != nil {
				return nil, errors.Wrapf(err, "decode ref %s", elem.ref.MarshalString())
			}

			// unmarshal the block
			if err := decodeBlk.UnmarshalBlock(dat); err != nil {
				return nil, errors.Wrapf(err, "unmarshal ref %s", elem.ref.MarshalString())
			}

			blk = decodeBlk
		} else if blk == nil {
			continue
		}

		// enqueue any sub-blocks
		if withSubBlocks, ok := blk.(block.BlockWithSubBlocks); ok {
			subBlks := withSubBlocks.GetSubBlocks()
			for _, subBlk := range subBlks {
				if subBlk != nil && !subBlk.IsNil() {
					stack = append(stack, stackElem{
						blk: subBlk,
					})
				}
			}
		}

		// if the block has no refs, continue.
		withRefs, ok := blk.(block.BlockWithRefs)
		if !ok {
			continue
		}

		blkRefs, err := withRefs.GetBlockRefs()
		if err != nil {
			return nil, errors.Wrap(err, "get block refs")
		}
		nextStackElems := make([]stackElem, 0, len(blkRefs))
		for refID, ref := range blkRefs {
			if ref.GetEmpty() {
				continue
			}
			nextStackElems = append(nextStackElems, stackElem{
				ref:  ref,
				ctor: withRefs.GetBlockRefCtor(refID),
			})
		}
		sort.SliceStable(nextStackElems, func(i, j int) bool {
			return nextStackElems[i].ref.LessThan(nextStackElems[j].ref)
		})
		nextStackElems = slices.CompactFunc(nextStackElems, func(a, b stackElem) bool {
			return a.ref.EqualsRef(b.ref)
		})
		stack = append(stack, nextStackElems...)
	}

	return destinationRef, nil
}
