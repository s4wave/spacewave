package dist_compiler_bundle

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// BundleManifestsKvfile packs the world and its manifests in traversal order
// for read locality. Only reachable blocks are included; prior build history
// in the backing store does not affect the result.
// The caller must finish writing the World before packing; accepted writes
// are fenced before the archive reads blocks from their backing store.
// The kvfile records the current format version, so a Volume opens it without
// migrating.
func BundleManifestsKvfile(
	ctx context.Context,
	le *logrus.Entry,
	kvfileWriter *kvfile.Writer,
	keys *store_kvkey.KVKey,
	blkEng *world_block.Engine,
) error {
	// Record the format version of the block keys.
	version := store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)
	if err := kvfileWriter.WriteValue(keys.GetFormatVersionKey(), bytes.NewReader(version)); err != nil {
		return err
	}

	// Raw block traversal must see every accepted write, including deferred ones.
	if _, err := blkEng.Sync(ctx); err != nil {
		return err
	}
	nextRootRef := blkEng.GetRootRef()

	// Write each reachable block once in stable traversal order.
	seen := make(map[string]struct{})
	walkWriteBlocks := func(bls *bucket_lookup.Cursor, ref *block.BlockRef, ctor block.Ctor) error {
		return bucket_lookup.WalkObjectBlocks(
			ctx,
			bucket_lookup.NewWalkObjectBlocksWithRef(ref, ctor),
			func(ent *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
				// Propagate walk errors and skip sub-blocks and empty refs.
				if ent.Err != nil {
					return false, ent.Err
				}
				if ent.IsSubBlock || ent.Ref.GetEmpty() {
					return true, nil
				}
				if !ent.Found {
					return false, errors.Wrap(block.ErrNotFound, ent.Ref.MarshalString())
				}

				// Write each first-seen block under its block key.
				rm, err := ent.Ref.MarshalKey()
				if err != nil {
					return false, err
				}
				if _, ok := seen[string(rm)]; ok {
					return true, nil
				}
				seen[string(rm)] = struct{}{}
				return true, kvfileWriter.WriteValue(keys.GetBlockKey(rm), bytes.NewReader(ent.Data))
			},
			bls.GetBucket(),
			bls.GetTransformer(),
			1, // Serial callbacks preserve traversal order and writer ownership.
			false,
		)
	}

	// Pack the World root followed by the referenced manifest DAGs.
	return blkEng.AccessWorldState(ctx, nextRootRef, func(bls *bucket_lookup.Cursor) error {
		// Write the World root's blocks into the kvfile.
		if err := walkWriteBlocks(bls, nextRootRef.GetRootRef(), world_block.NewWorldBlock); err != nil {
			return err
		}

		// Open a read transaction to enumerate the manifest objects.
		wtx, err := blkEng.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer wtx.Discard()

		// Pack each manifest object's block DAG after the World root.
		return world_types.IterateObjectsWithType(ctx, wtx, bldr_manifest_world.ManifestTypeID, func(objKey string) (bool, error) {
			// Load the manifest object and its root ref.
			obj, err := world.MustGetObject(ctx, wtx, objKey)
			defer world.ReleaseObjectState(obj)
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

			// Pack the manifest block DAG into the kvfile.
			rootBls, err := bls.FollowRef(ctx, rootRef)
			if err != nil {
				return false, err
			}
			defer rootBls.Release()
			err = walkWriteBlocks(rootBls, rootRef.GetRootRef(), bldr_manifest.NewManifestBlock)
			return err == nil, err
		})
	})
}
