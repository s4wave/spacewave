package bldr_manifest_world

import (
	"context"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// AccessManifest accesses the FS associated with a manifest from a world.
func AccessManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessFunc world.AccessWorldStateFunc,
	manifestRef *bucket.ObjectRef,
	cb func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error,
) error {
	return accessFunc(ctx, manifestRef, func(bls *bucket_lookup.Cursor) error {
		return bldr_manifest.AccessManifest(ctx, le, bls, cb)
	})
}
