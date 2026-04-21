package bldr_manifest_world

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// DeepCopyManifest copies a manifest by fully re-creating the manifest using filesystem copies.
// Completely re-encodes the manifest with a new underlying block graph.
// Useful when copying between two locations with different transform configs.
//
// note: the transform config and object ref will be based on the reference
// contained within the cursor after calling destAccess(nil)
func DeepCopyManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessSrcManifest world.AccessWorldStateFunc,
	srcManifestRef *bucket.ObjectRef,
	destWorldState world.WorldState,
	destAccess world.AccessWorldStateFunc,
	destObjectKey string,
	destLinkObjKeys []string,
	opPeerID peer.ID,
	ts *timestamp.Timestamp,
) (*bldr_manifest.Manifest, *bucket.ObjectRef, error) {
	var outManifest *bldr_manifest.Manifest
	var outRef *bucket.ObjectRef
	writeErr := AccessManifest(
		ctx,
		le,
		accessSrcManifest,
		srcManifestRef,
		func(
			ctx context.Context,
			bls *bucket_lookup.Cursor,
			bcs *block.Cursor,
			manifest *bldr_manifest.Manifest,
			distFS *unixfs.FSHandle,
			assetsFS *unixfs.FSHandle,
		) error {
			// distIoFS := unixfs_iofs.NewFS(ctx, distFS)
			// assetsIoFS := unixfs_iofs.NewFS(ctx, assetsFS)
			writeTs := ts.AsTime()
			if writeTs.IsZero() {
				writeTs = time.Now()
			}

			distBfs := unixfs_billy.NewBillyFilesystem(ctx, distFS, "", writeTs)
			assetsBfs := unixfs_billy.NewBillyFilesystem(ctx, assetsFS, "", writeTs)

			// note: the transform config and object ref will be based on the
			// reference contained within the cursor after calling destAccess(nil)
			var err error
			outManifest, outRef, err = CommitManifest(
				ctx,
				le,
				destWorldState,
				destAccess,
				manifest.GetMeta(),
				manifest.GetEntrypoint(),
				distBfs,
				assetsBfs,
				destObjectKey,
				destLinkObjKeys,
				opPeerID,
				ts,
			)
			return err
		},
	)
	return outManifest, outRef, writeErr
}
