package bldr_manifest_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// ReleaseManifestTime stamps every UnixFS node of a released Manifest. It is a
// constant so the encoding is a pure function of the logical content: the same
// files always produce the same blocks, and a Manifest that did not change
// keeps the same dist and assets references.
var ReleaseManifestTime = timestamp.New(time.Unix(1, 0).UTC())

// ReleaseManifestAccess writes new manifest objects with the gzip transform
// understood by both native and browser readers. Existing references retain
// their own transforms; only new payloads use this release policy.
func ReleaseManifestAccess(le *logrus.Entry, storage world.WorldStorage) world.AccessWorldStateFunc {
	return func(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
		// Read an existing reference with the transform it already has.
		if !ref.GetEmpty() {
			return storage.AccessWorldState(ctx, ref, cb)
		}

		// Open the World's storage cursor for the new object.
		base, err := storage.BuildStorageCursor(ctx)
		if err != nil {
			return err
		}
		defer base.Release()

		// Build the gzip transformer for the new payload.
		conf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
		if err != nil {
			return err
		}
		xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, base.GetStepFactorySet(), conf)
		if err != nil {
			return err
		}

		// Write through a cursor that applies the transformer, into the store
		// that owns the base cursor's writes.
		cursor := bucket_lookup.NewCursor(ctx, nil, le, base.GetStepFactorySet(), base.GetBucket(), xfrm, nil, base.GetOpArgs(), conf)
		defer cursor.Release()
		cursor.SetTransactionStore(base.GetBlockStore())
		return cb(cursor)
	}
}

// EncodeReleaseManifest encodes the source Manifest into the destination World
// with the release policy and returns the encoded Manifest and its reference.
// The encoding is not linked into the World; dest owns its blocks until the
// caller links it.
//
// meta carries the revision of the encoded Manifest. The encoded blocks of
// unchanged files and directories equal the ones already encoded anywhere, so
// storing them adds only new blocks.
func EncodeReleaseManifest(
	ctx context.Context,
	le *logrus.Entry,
	src world.AccessWorldStateFunc,
	srcRef *bucket.ObjectRef,
	meta *bldr_manifest.ManifestMeta,
	dest world.WorldStorage,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	var out *bldr_manifest.Manifest
	var ref *bucket.ObjectRef
	err := AccessManifest(ctx, le, src, srcRef, func(
		ctx context.Context,
		_ *bucket_lookup.Cursor,
		_ *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		// Adapt the source file trees for encoding.
		distBfs := manifestBillyFS(ctx, manifest.GetDistFsRef(), distFS)
		assetsBfs := manifestBillyFS(ctx, manifest.GetAssetsFsRef(), assetsFS)

		// Encode the Manifest with the destination metadata.
		out = &bldr_manifest.Manifest{
			Meta:       meta,
			Entrypoint: manifest.GetEntrypoint(),
			Deps:       manifest.GetDeps(),
		}
		var err error
		ref, err = world.AccessObject(ctx, ReleaseManifestAccess(le, dest), nil, func(bcs *block.Cursor) error {
			return bldr_manifest.CreateManifestWithBilly(ctx, bcs, out, distBfs, assetsBfs, ReleaseManifestTime)
		})
		return errors.Wrap(err, "encode release manifest")
	})
	return out, ref, err
}

// manifestBillyFS adapts a Manifest file tree for encoding, or returns nil for a
// tree the Manifest does not have, which encodes as an empty directory.
func manifestBillyFS(ctx context.Context, ref *block.BlockRef, handle *unixfs.FSHandle) billy.Filesystem {
	if ref.GetEmpty() {
		return nil
	}
	return unixfs_billy.NewBillyFilesystem(ctx, handle, "", ReleaseManifestTime.AsTime())
}
