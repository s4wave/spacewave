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
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// BundleManifestsKvfile packs the world and its manifests in traversal order
// for read locality. Only reachable blocks are included; prior build history
// in the backing store does not affect the result.
func BundleManifestsKvfile(
	ctx context.Context,
	le *logrus.Entry,
	kvfileWriter *kvfile.Writer,
	kvfileBlockPrefix []byte,
	blkEng *world_block.Engine,
) error {
	nextRootRef := blkEng.GetRootRef()
	seen := make(map[string]struct{})
	walkWriteBlocks := func(bls *bucket_lookup.Cursor, ref *block.BlockRef, ctor block.Ctor) error {
		return bucket_lookup.WalkObjectBlocks(
			ctx,
			bucket_lookup.NewWalkObjectBlocksWithRef(ref, ctor),
			func(ent *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
				if ent.Err != nil {
					return false, ent.Err
				}
				if ent.IsSubBlock || ent.Ref.GetEmpty() {
					return true, nil
				}
				if !ent.Found {
					return false, errors.Wrap(block.ErrNotFound, ent.Ref.MarshalString())
				}
				key := ent.Ref.MarshalString()
				if _, ok := seen[key]; ok {
					return true, nil
				}
				seen[key] = struct{}{}
				outKey := append(bytes.Clone(kvfileBlockPrefix), key...)
				return true, kvfileWriter.WriteValue(outKey, bytes.NewReader(ent.Data))
			},
			bls.GetBucket(),
			bls.GetTransformer(),
			1, // Serial callbacks preserve traversal order and writer ownership.
			false,
		)
	}

	return blkEng.AccessWorldState(ctx, nextRootRef, func(bls *bucket_lookup.Cursor) error {
		if err := walkWriteBlocks(bls, nextRootRef.GetRootRef(), world_block.NewWorldBlock); err != nil {
			return err
		}
		wtx, err := blkEng.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer wtx.Discard()

		return world_types.IterateObjectsWithType(ctx, wtx, bldr_manifest_world.ManifestTypeID, func(objKey string) (bool, error) {
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
