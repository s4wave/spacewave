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

// DeepCopyManifest re-encodes a Manifest into a destination World. The
// destination access function determines its transform and object reference.
// destManifestMeta overrides the copied metadata when non-nil.
func DeepCopyManifest(
	ctx context.Context,
	le *logrus.Entry,
	accessSrcManifest world.AccessWorldStateFunc,
	srcManifestRef *bucket.ObjectRef,
	destManifestMeta *bldr_manifest.ManifestMeta,
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
			// Select destination metadata while preserving source metadata by default.
			manifestMeta := destManifestMeta
			if manifestMeta == nil {
				manifestMeta = manifest.GetMeta()
			}

			// Adapt both source filesystems to the destination commit interface.
			writeTs := ts.AsTime()
			if writeTs.IsZero() {
				writeTs = time.Now()
			}

			distBfs := unixfs_billy.NewBillyFilesystem(ctx, distFS, "", writeTs)
			assetsBfs := unixfs_billy.NewBillyFilesystem(ctx, assetsFS, "", writeTs)

			// Re-encode the logical content into the destination World.
			var err error
			outManifest, outRef, err = CommitManifest(
				ctx,
				le,
				destWorldState,
				destAccess,
				manifestMeta,
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
