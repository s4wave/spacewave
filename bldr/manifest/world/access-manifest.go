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
	if manifestRef != nil && manifestRef.GetBucketId() != "" {
		return accessFunc(ctx, nil, func(root *bucket_lookup.Cursor) error {
			if manifestRef.GetBucketId() == root.GetOpArgs().GetBucketId() {
				le.WithField("bucket-id", manifestRef.GetBucketId()).Debug("following manifest ref in current bucket")
				manifest, err := root.FollowRef(ctx, manifestRef)
				if err != nil {
					return err
				}
				defer manifest.Release()
				le.WithField("bucket-id", manifestRef.GetBucketId()).Debug("accessing followed manifest in current bucket")
				return bldr_manifest.AccessManifest(ctx, le, manifest, cb)
			}

			opArgs := root.GetOpArgs()
			opArgs.BucketId = manifestRef.GetBucketId()
			opArgs.VolumeId = ""
			le.WithFields(logrus.Fields{
				"bucket-id":      manifestRef.GetBucketId(),
				"root-bucket-id": root.GetOpArgs().GetBucketId(),
			}).Debug("following manifest ref in external bucket")
			manifest, err := root.FollowRefWithOpArgs(ctx, manifestRef, opArgs)
			if err != nil {
				return err
			}
			defer manifest.Release()
			le.WithField("bucket-id", manifestRef.GetBucketId()).Debug("accessing followed manifest in external bucket")
			return bldr_manifest.AccessManifest(ctx, le, manifest, cb)
		})
	}

	return accessFunc(ctx, manifestRef, func(bls *bucket_lookup.Cursor) error {
		return bldr_manifest.AccessManifest(ctx, le, bls, cb)
	})
}

// AccessStartupManifest accesses a startup manifest with local-only lookup
// reads so unavailable optional startup refs fail fast instead of waiting on
// network lookup.
func AccessStartupManifest(
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
	if manifestRef == nil || manifestRef.GetEmpty() {
		return AccessManifest(ctx, le, accessFunc, manifestRef, cb)
	}

	return accessFunc(ctx, nil, func(root *bucket_lookup.Cursor) error {
		manifestCursor, err := followStartupManifestRef(ctx, root, manifestRef)
		if err != nil {
			return err
		}
		defer manifestCursor.Release()

		localManifestCursor := manifestCursor.CloneWithLocalOnlyReads()
		defer localManifestCursor.Release()

		return bldr_manifest.AccessManifest(ctx, le, localManifestCursor, cb)
	})
}
