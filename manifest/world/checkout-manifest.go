package bldr_manifest_world

import (
	"context"

	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// CheckoutManifest checks out the manifest to paths on disk.
//
// If either of the paths are empty, they will be skipped.
// If manifestRef is nil, will use the reference defaulted to by accessFunc.
func CheckoutManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessFunc world.AccessWorldStateFunc,
	manifestRef *bucket.ObjectRef,
	outDistPath, outAssetsPath string,
	deleteMode unixfs_sync.DeleteMode,
	filterDistCb unixfs_sync.FilterCb,
	filterAssetsCb unixfs_sync.FilterCb,
) (*manifest.Manifest, error) {
	var outManifest *manifest.Manifest
	err := AccessManifest(ctx, le, accessFunc, manifestRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		outManifest = manifest

		// checkout the plugin dist unixfs to the disk.
		if outDistPath != "" {
			if err := unixfs_sync.Sync(
				ctx,
				outDistPath,
				distFS,
				deleteMode,
				filterDistCb,
			); err != nil {
				return err
			}
		}

		// check out the plugin assets unixfs to the disk.
		if outAssetsPath != "" {
			if err := unixfs_sync.Sync(
				ctx,
				outAssetsPath,
				assetsFS,
				deleteMode,
				filterAssetsCb,
			); err != nil {
				return err
			}
		}

		// success
		return nil
	})

	return outManifest, err
}
