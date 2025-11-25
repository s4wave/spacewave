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
	"github.com/go-git/go-billy/v5"
	"github.com/sirupsen/logrus"
)

// CheckoutManifestToBilly checks out the manifest to a billy filesystem.
//
// If either of the filesystems are nil, they will be skipped.
// If manifestRef is nil, will use the reference defaulted to by accessFunc.
func CheckoutManifestToBilly(
	ctx context.Context,
	le *logrus.Entry,
	accessFunc world.AccessWorldStateFunc,
	manifestRef *bucket.ObjectRef,
	distFs, assetsFs billy.Filesystem,
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

		// sync the plugin dist unixfs to the billy filesystem.
		if distFs != nil {
			if err := unixfs_sync.SyncToBilly(
				ctx,
				distFs,
				distFS,
				deleteMode,
				filterDistCb,
			); err != nil {
				return err
			}
		}

		// sync the plugin assets unixfs to the billy filesystem.
		if assetsFs != nil {
			if err := unixfs_sync.SyncToBilly(
				ctx,
				assetsFs,
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
