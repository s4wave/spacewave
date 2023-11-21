package bldr_manifest_world

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/world"
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
