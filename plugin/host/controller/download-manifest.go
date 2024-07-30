package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/pkg/errors"
)

// execDownloadManifest executes downloading the manifest fetched from FetchManifest to the world.
func (t *executePlugin) execDownloadManifest(ctx context.Context, manifestValue *bldr_manifest.FetchManifestValue) error {
	le := t.c.le
	meta := manifestValue.GetManifestRef().GetMeta()
	le.Debugf("starting plugin manifest downloader: %s", meta.GetManifestId())

	// get world state handle
	ws, err := t.c.getWorldState(ctx)
	if err != nil {
		return err
	}

	pluginManifestRef := manifestValue.GetManifestRef()
	if err := pluginManifestRef.Validate(); err != nil {
		return errors.Wrap(err, "download plugin returned invalid manifest ref")
	}
	manifestRef := pluginManifestRef.ManifestRef
	if pluginManifestRef.GetEmpty() || manifestRef.GetEmpty() {
		return errors.New("download plugin returned empty manifest ref")
	}

	if t.c.conf.GetDisableStoreManifest() {
		pluginManifestRef.Meta.Logger(le).Debug("skipping storing downloaded manifest")
		return nil
	}

	// use an empty volume ID to allow cross-volume lookup of manifest contents
	var pluginHostBucketID string
	le = pluginManifestRef.Meta.Logger(le)

	// access manifest
	var pluginManifest *bldr_manifest.Manifest
	var manifestBucketID string
	var wroteManifestRef *bucket.ObjectRef

	le.Debug("accessing fetched manifest")
	err = ws.AccessWorldState(ctx, nil, func(worldCursor *bucket_lookup.Cursor) error {
		opArgs := &bucket.BucketOpArgs{}
		pluginHostBucketID = worldCursor.GetOpArgs().GetBucketId()
		if refBucketID := manifestRef.GetBucketId(); refBucketID != "" {
			// use the bucket id from the ref, if any.
			opArgs.BucketId = refBucketID
		} else {
			// if there was no bucket id specified, use the plugin host bucket.
			opArgs.BucketId = pluginHostBucketID
		}

		manifestCursor, err := worldCursor.FollowRefWithOpArgs(ctx, manifestRef, opArgs)
		if err != nil {
			return err
		}
		defer manifestCursor.Release()

		_, bcs := manifestCursor.BuildTransaction(nil)
		pluginManifest, err = bldr_manifest.UnmarshalManifest(ctx, bcs)
		if err != nil {
			return err
		}
		if manifestID := pluginManifest.GetMeta().GetManifestId(); manifestID != meta.GetManifestId() {
			return errors.Errorf(
				"tried to fetch manifest %s but returned manifest %s",
				meta.GetManifestId(),
				manifestID,
			)
		}
		if err := pluginManifest.Validate(); err != nil {
			return err
		}
		manifestBucketID = manifestCursor.GetOpArgs().GetBucketId()

		// if the manifest is located in a different bucket, copy it over.
		if manifestBucketID == pluginHostBucketID || t.c.conf.GetDisableCopyManifest() {
			wroteManifestRef = manifestRef.Clone()
			return nil
		}

		le.Infof("copying manifest contents from bucket %s to %s", manifestBucketID, pluginHostBucketID)

		writeBaseRef := manifestCursor.GetRef().Clone()
		writeBaseRef.BucketId = pluginHostBucketID

		writeCursor, err := worldCursor.FollowRef(ctx, writeBaseRef)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Wrap(err, "copy manifest: construct write cursor")
		}
		defer writeCursor.Release()

		// TODO: This can skip some blocks required! Needs updating!
		// TODO: see dist/compiler/bundle.go => copying block that was not part of the world state or any manifest
		concurrentLimit := t.c.conf.GetFetchConcurrency()
		wroteManifestRef, err = bucket_lookup.CopyObjectToBucket(
			ctx,
			writeCursor,
			manifestCursor,
			bldr_manifest.NewManifestBlock,
			int(concurrentLimit),
			// set true to skip block sub-graphs if they already existed
			true,
			nil,
		)
		if err == nil {
			le.Infof("completed copying manifest contents to %s", pluginHostBucketID)
		} else {
			le.WithError(err).Warnf("failed to copy manifest contents to %s", pluginHostBucketID)
		}

		return err
	})
	if err != nil {
		return err
	}

	// update the manifestRef with the new root reference
	storedManifestRef := pluginManifestRef.CloneVT()
	storedManifestRef.ManifestRef = wroteManifestRef

	// check if the stored manifest is equivalent (skip store)
	manifestKey := bldr_manifest.NewManifestKey(t.c.objKey, pluginManifest.GetMeta())
	prevManifestState, prevManifestFound, err := ws.GetObject(ctx, manifestKey)
	if err != nil {
		return err
	}

	var skipRegisterManifest bool
	if prevManifestFound {
		prevRootRef, _, err := prevManifestState.GetRootRef(ctx)
		if err != nil {
			return err
		}
		skipRegisterManifest = prevRootRef.EqualsRef(wroteManifestRef)
	}

	// submit operation to update + link plugin manifest
	if !skipRegisterManifest {
		le.Debug("registering fetched plugin manifest")
		err = bldr_manifest_world.ExStoreManifestOp(
			ctx,
			ws,
			t.c.peerID,
			manifestKey,
			[]string{t.c.objKey},
			storedManifestRef,
		)
		if err != nil {
			return err
		}
	}

	le.Infof("successfully fetched manifest for plugin: %s", t.pluginID)
	return nil
}
