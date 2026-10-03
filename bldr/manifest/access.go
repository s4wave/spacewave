package bldr_manifest

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
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
	// Decode the manifest from the bucket cursor and record its filesystem roots.
	_, bcs := bls.BuildTransaction(nil)
	le.Debug("unmarshalling manifest")
	manifest, err := UnmarshalManifest(ctx, bcs)
	if err != nil {
		return err
	}
	le.WithFields(logrus.Fields{
		"assets-fs-ref": manifest.GetAssetsFsRef().MarshalLog(),
		"dist-fs-ref":   manifest.GetDistFsRef().MarshalLog(),
		"entrypoint":    manifest.GetEntrypoint(),
	}).Debug("manifest unmarshalled")

	// build unixfs_block_fs backed by the distribution fs
	distBls := bls.Clone()
	defer distBls.Release()

	// Build the distribution filesystem over the manifest distribution root.
	distBls.SetRootRef(manifest.GetDistFsRef())
	le.Debug("building manifest dist filesystem handle")
	distWriter := unixfs_block_fs.NewFSWriter()
	distFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, distBls, distWriter)
	distWriter.SetFS(distFS)
	defer distFS.Release()

	// Expose the distribution filesystem through a retained UnixFS handle.
	distUfs, err := unixfs.NewFSHandle(distFS)
	if err != nil {
		return err
	}
	defer distUfs.Release()

	// build unixfs_block_fs backed by the assets fs
	assetsBls := bls.Clone()
	defer assetsBls.Release()

	// Build the assets filesystem over the manifest assets root.
	assetsBls.SetRootRef(manifest.GetAssetsFsRef())
	le.Debug("building manifest assets filesystem handle")
	assetsWriter := unixfs_block_fs.NewFSWriter()
	assetsFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, assetsBls, assetsWriter)
	assetsWriter.SetFS(assetsFS)
	defer assetsFS.Release()

	// Expose the assets filesystem through a retained UnixFS handle.
	assetsUfs, err := unixfs.NewFSHandle(assetsFS)
	if err != nil {
		return err
	}
	defer assetsUfs.Release()

	// Run the manifest callback while both filesystem handles remain retained.
	le.Debug("calling manifest access callback")
	return cb(ctx, bls, bcs, manifest, distUfs, assetsUfs)
}
