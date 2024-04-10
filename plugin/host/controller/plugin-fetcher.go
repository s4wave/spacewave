package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/retry"
	"github.com/pkg/errors"
)

// pluginManifestFetcher tracks fetching plugin manifests.
type pluginManifestFetcher struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// resultPromise contains the result of the fetcher
	resultPromise *promise.PromiseContainer[*bldr_manifest.FetchManifestValue]
}

// newPluginManifestFetcher constructs a new plugin manifest fetcher routine.
func (c *Controller) newPluginManifestFetcher(pluginID string) (keyed.Routine, *pluginManifestFetcher) {
	tr := &pluginManifestFetcher{
		c:             c,
		pluginID:      pluginID,
		resultPromise: promise.NewPromiseContainer[*bldr_manifest.FetchManifestValue](),
	}
	return tr.execute, tr
}

// execute executes the plugin fetcher.
func (t *pluginManifestFetcher) execute(ctx context.Context) error {
	// determine host plugin platform id
	hostPluginPlatformID, err := t.c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return err
	}

	meta := &bldr_manifest.ManifestMeta{
		ManifestId: t.pluginID,
		PlatformId: hostPluginPlatformID,
	}

	// If AlwaysFetchManifest is enabled, keep a FetchManifest directive running.
	// If the manifest is updated, the plugin fetcher will be restarted.
	alwaysFetchManifest := t.c.conf.GetAlwaysFetchManifest()
	if alwaysFetchManifest {
		_, fetchRef, err := t.c.bus.AddDirective(bldr_manifest.NewFetchManifest(meta), nil)
		if err != nil {
			return err
		}
		defer fetchRef.Release()
	}

	backoffConf := t.c.conf.GetFetchBackoff().CloneVT()
	if backoffConf == nil {
		backoffConf = &backoff.Backoff{}
	}
	if backoffConf.BackoffKind == 0 {
		if backoffConf.Exponential == nil {
			backoffConf.Exponential = &backoff.Exponential{}
		}
		backoffConf.BackoffKind = backoff.BackoffKind_BackoffKind_EXPONENTIAL
		backoffConf.Exponential.MaxInterval = 4200
	}

	bo := backoffConf.Construct()
	return retry.Retry(
		ctx,
		t.c.le.WithField("plugin-id", t.pluginID),
		func(ctx context.Context, success func()) error {
			resultProm := promise.NewPromise[*bldr_manifest.FetchManifestValue]()
			t.resultPromise.SetPromise(resultProm)
			resp, err := t.fetchManifest(ctx, meta)
			if err == nil {
				success()
			}
			if err != context.Canceled {
				resultProm.SetResult(resp, err)
			}
			if err == nil && alwaysFetchManifest {
				// Keep the FetchManifest directive running until the context is canceled.
				<-ctx.Done()
				err = context.Canceled
			}
			return err
		},
		bo,
	)
}

// fetchManifest attempts to fetch the manifest.
func (t *pluginManifestFetcher) fetchManifest(ctx context.Context, meta *bldr_manifest.ManifestMeta) (*bldr_manifest.FetchManifestValue, error) {
	le := t.c.le
	le.Debugf("starting plugin manifest fetcher: %s", meta.GetManifestId())

	// get world state handle
	ws, err := t.c.getWorldState(ctx)
	if err != nil {
		return nil, err
	}

	// fetch the manifest for this plugin
	// wait until the plugin has been fetched
	res, err := bldr_manifest.ExFetchManifest(ctx, t.c.bus, meta, false)
	if err != nil {
		return nil, err
	}
	pluginManifestRef := res.GetManifestRef()
	if err := pluginManifestRef.Validate(); err != nil {
		return nil, errors.Wrap(err, "fetch plugin returned invalid manifest ref")
	}
	manifestRef := pluginManifestRef.ManifestRef
	if pluginManifestRef.GetEmpty() || manifestRef.GetEmpty() {
		return nil, errors.New("fetch plugin returned empty manifest ref")
	}

	if t.c.conf.GetDisableStoreManifest() {
		pluginManifestRef.Meta.Logger(le).Debug("skipping storing fetched manifest")
		return bldr_manifest.NewFetchManifestValue(pluginManifestRef), nil
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
			opArgs.BucketId = refBucketID
		} else {
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

		le.Infof("copying manifest from bucket %s to %s", manifestBucketID, pluginHostBucketID)
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

		concurrentLimit := t.c.conf.GetFetchConcurrency()
		wroteManifestRef, err = bucket_lookup.CopyObjectToBucket(
			ctx,
			writeCursor,
			manifestCursor,
			bldr_manifest.NewManifestBlock,
			int(concurrentLimit),
			nil,
		)
		if err == nil {
			le.Infof("completed copying manifest to %s", pluginHostBucketID)
		} else {
			le.WithError(err).Warnf("failed to copy manifest to %s", pluginHostBucketID)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	// update the manifestRef with the new root reference
	storedManifestRef := pluginManifestRef.CloneVT()
	storedManifestRef.ManifestRef = wroteManifestRef

	// check if the stored manifest is equivalent (skip store)
	manifestKey := bldr_manifest.NewManifestKey(t.c.objKey, pluginManifest.GetMeta())
	prevManifestState, prevManifestFound, err := ws.GetObject(ctx, manifestKey)
	if err != nil {
		return nil, err
	}
	var skipRegisterManifest bool
	if prevManifestFound {
		prevRootRef, _, err := prevManifestState.GetRootRef(ctx)
		if err != nil {
			return nil, err
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
			return nil, err
		}
	}

	le.Infof("successfully fetched manifest for plugin: %s", t.pluginID)
	return bldr_manifest.NewFetchManifestValue(storedManifestRef), nil
}
