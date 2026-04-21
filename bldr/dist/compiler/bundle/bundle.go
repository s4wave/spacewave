package dist_compiler_bundle

import (
	"bytes"
	"context"
	"sync"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// BundleManifestsKvfile copies the blocks to the kvfile in the order that they
// are read after first traversing the world state blocks followed by the blocks
// for each manifest.
//
// Ideally this will cover all of the blocks in the storage, however, if there
// are any remaining blocks not found after this process, those are copied last.
func BundleManifestsKvfile(
	ctx context.Context,
	le *logrus.Entry,
	kvfileWriter *kvfile.Writer,
	kvfileBlockPrefix []byte,
	blkEng *world_block.Engine,
	kvtxVolStore kvtx.Store,
	kvtxVolBlockPrefix []byte,
) error {
	nextRootRef := blkEng.GetRootRef()
	// nextRootRefStr := nextRootRef.MarshalString()

	// Ensure we do not process duplicate blocks by tracking which blocks were seen.
	// use a sync.Map since this is the exact situation it is meant for
	// key: string (BlockRef)
	// value: bool (seen)
	var seenBlocks sync.Map

	// TODO: track parents for debugging. not necessary, remove eventually.
	parents := make(map[string]*bucket_lookup.WalkObjectBlocksEntry)

	walkWriteBlocks := func(bls *bucket_lookup.Cursor, rootEnt *bucket_lookup.WalkObjectBlocksEntry) error {
		return bucket_lookup.WalkObjectBlocks(
			ctx,
			rootEnt,
			func(ent *bucket_lookup.WalkObjectBlocksEntry) (cntu bool, err error) {
				err = ent.Err
				if err == nil && !ent.Found && !ent.IsSubBlock && !ent.Ref.GetEmpty() {
					err = errors.Wrap(block.ErrNotFound, ent.Ref.MarshalString())
				}
				cntu = err == nil

				if err != nil || ent.IsSubBlock || !ent.Found || ent.Ref.GetEmpty() || len(ent.Data) == 0 {
					// skip this block since it is not found or a sub-block or empty
					return
				}

				// skip copying if we already saw this block
				refStr := ent.Ref.MarshalString()
				_, seen := seenBlocks.LoadOrStore(refStr, true)
				if seen {
					return
				}

				// TODO
				parent := parents[refStr]
				// le.Debugf("COPY BLOCK: %s: len(%d): %#v", refStr, len(ent.Data), ent.Blk)
				if blb, blbOk := ent.Blk.(*blob.Blob); blbOk && blb.GetTotalSize() < 100 {
					le.Debugf("COPY SMALL BLOCK: %s: parent(%v) len(%d): %#v", refStr, parent, len(ent.Data), ent.Blk)
				}
				if withRefs, withRefsOk := ent.Blk.(block.BlockWithRefs); withRefsOk {
					outRefs, err := withRefs.GetBlockRefs()
					if err != nil {
						return false, err
					}
					for _, outRef := range outRefs {
						parents[outRef.MarshalString()] = ent
					}
				}

				// copy to the kvfile
				key := bytes.Join([][]byte{kvfileBlockPrefix, []byte(refStr)}, nil)
				err = kvfileWriter.WriteValue(key, bytes.NewReader(ent.Data))
				cntu = err == nil
				return
			},
			bls.GetBucket(),
			bls.GetTransformer(),
			1, // 1 concurrency to get correct order
			false,
		)
	}

	// TODO: Replace the Walk functions here with analyzing the GC graph.

	// The only values we will be using in this kvfile are blocks for the world engine & manifests.
	// Walk the world block store and manifests and store the blocks in that order.
	// This optimizes the order of the values in the file to the order they will be accessed.
	// This means that grabbing a 1MB chunk of the file is more likely to have related data.
	// This is a significant optimization over key-sorted-order values.
	return blkEng.AccessWorldState(ctx, nextRootRef, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		worldRoot, err := world_block.UnmarshalWorld(ctx, bcs)
		if err != nil {
			return err
		}

		// Write the blocks for the world k/v store
		if err := walkWriteBlocks(bls, bucket_lookup.NewWalkObjectBlocksWithBlock(worldRoot)); err != nil {
			return err
		}

		// Access the world contents
		wtx, err := blkEng.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer wtx.Discard()

		// Lookup the list of manifests and write them to the store.
		err = world_types.IterateObjectsWithType(ctx, wtx, bldr_manifest_world.ManifestTypeID, func(objKey string) (bool, error) {
			obj, err := world.MustGetObject(ctx, wtx, objKey)
			if err != nil {
				return false, err
			}

			rootRef, _, err := obj.GetRootRef(ctx)
			if err != nil {
				return false, err
			}
			if rootRef.GetEmpty() {
				return true, nil
			}

			// copy the manifest
			rootBls, err := bls.FollowRef(ctx, rootRef)
			if err != nil {
				return false, err
			}
			defer rootBls.Release()

			_, bcs := rootBls.BuildTransaction(nil)
			manifest, err := bldr_manifest.UnmarshalManifest(ctx, bcs)
			if err != nil {
				return false, err
			}

			if err := walkWriteBlocks(rootBls, bucket_lookup.NewWalkObjectBlocksWithBlock(manifest)); err != nil {
				return false, err
			}

			return true, nil
		})
		if err != nil {
			return err
		}

		wtx.Discard()

		// Finally, copy (& log) any blocks that we missed above.
		kvtxTx, err := kvtxVolStore.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer kvtxTx.Discard()

		return kvtxTx.ScanPrefix(ctx, kvtxVolBlockPrefix, func(key, value []byte) error {
			blockKey := key[len(kvtxVolBlockPrefix):]
			ref, err := block.UnmarshalBlockRefB58(string(blockKey))
			if err != nil {
				return errors.Wrapf(err, "invalid block ref key: %v", blockKey)
			}
			if ref.GetEmpty() {
				return nil
			}

			refStr := ref.MarshalString()
			_, seen := seenBlocks.LoadOrStore(refStr, true)
			if seen {
				return nil
			}

			// write the block to the store
			le.Debugf("copying block that was not part of the world state or any manifest: %v", refStr)
			kvfileBlockKey := bytes.Join([][]byte{kvfileBlockPrefix, []byte(refStr)}, nil)
			return kvfileWriter.WriteValue(kvfileBlockKey, bytes.NewReader(value))
		})
	})
}
