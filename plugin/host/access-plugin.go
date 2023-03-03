package plugin_host

import (
	"context"

	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// AccessPluginManifest accesses the FS associated with a plugin manifest.
func AccessPluginManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessFunc world.AccessWorldStateFunc,
	manifestRef *bucket.ObjectRef,
	cb func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *plugin.PluginManifest,
		distFS *unixfs.FS,
		assetsFS *unixfs.FS,
	) error,
) error {
	return accessFunc(ctx, manifestRef, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		manifest, err := plugin.UnmarshalPluginManifest(bcs)
		if err != nil {
			return err
		}

		// build unixfs_block_fs backed by the distribution fs
		distBls := bls.Clone()
		defer distBls.Release()
		distBls.SetRootRef(manifest.GetDistFsRef())
		distWriter := unixfs_block_fs.NewFSWriter()
		distFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, distBls, distWriter)
		distWriter.SetFS(distFS)
		defer distFS.Release()
		distUfs := unixfs.NewFS(ctx, le, distFS, nil)
		defer distUfs.Release()

		// build unixfs_block_fs backed by the assets fs
		assetsBls := bls.Clone()
		defer assetsBls.Release()
		assetsBls.SetRootRef(manifest.GetAssetsFsRef())
		assetsWriter := unixfs_block_fs.NewFSWriter()
		assetsFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, assetsBls, assetsWriter)
		assetsWriter.SetFS(assetsFS)
		defer assetsFS.Release()
		assetsUfs := unixfs.NewFS(ctx, le, assetsFS, nil)
		defer assetsUfs.Release()

		return cb(ctx, bls, bcs, manifest, distUfs, assetsUfs)
	})
}
