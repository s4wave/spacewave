package bldr_manifest

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	"github.com/sirupsen/logrus"
)

// AccessManifest accesses the FS associated with a manifest from a cursor.
func AccessManifest(
	ctx context.Context,
	le *logrus.Entry,
	bls *bucket_lookup.Cursor,
	cb func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error,
) error {
	_, bcs := bls.BuildTransaction(nil)
	manifest, err := UnmarshalManifest(ctx, bcs)
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
	distUfs, err := unixfs.NewFSHandle(distFS)
	if err != nil {
		return err
	}
	defer distUfs.Release()

	// build unixfs_block_fs backed by the assets fs
	assetsBls := bls.Clone()
	defer assetsBls.Release()
	assetsBls.SetRootRef(manifest.GetAssetsFsRef())
	assetsWriter := unixfs_block_fs.NewFSWriter()
	assetsFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, assetsBls, assetsWriter)
	assetsWriter.SetFS(assetsFS)
	defer assetsFS.Release()
	assetsUfs, err := unixfs.NewFSHandle(assetsFS)
	if err != nil {
		return err
	}
	defer assetsUfs.Release()

	return cb(ctx, bls, bcs, manifest, distUfs, assetsUfs)
}
