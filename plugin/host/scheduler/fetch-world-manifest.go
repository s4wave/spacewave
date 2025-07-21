package plugin_host_scheduler

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/sirupsen/logrus"
)

type storeFetchedManifestsKey struct {
	valueID  uint32
	refIndex int
}

// execute executes the tracker.
func (t *pluginInstance) execFetchWorldManifest(ctx context.Context, hosts *pluginHostSet) error {
	// wait for hosts set
	if hosts == nil {
		return nil
	}

	platformIDs := hosts.toPlatformIDs()
	t.le.
		WithField("platform-ids", platformIDs).
		Debugf("starting fetch plugin manifests")

	// If configured, store manifests in the world.
	storeManifests := !t.c.conf.GetDisableStoreManifest()
	var handler directive.ReferenceHandler
	if storeManifests {
		// Keyed set of FetchManifestValue store routines.
		storeFetchedManifests := keyed.NewKeyedWithLogger(
			t.newManifestFetchValueStorer,
			t.le,
			keyed.WithRetry[storeFetchedManifestsKey, *fetchManifestValueStorer](&backoff.Backoff{}),
		)
		storeFetchedManifests.SetContext(ctx, false)

		handler = directive.NewTypedCallbackHandler(
			// value added
			func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				manifestValue := av.GetValue()
				for i, manifestRef := range manifestValue.GetManifestRefs() {
					if err := manifestRef.Validate(); err != nil {
						t.le.WithError(err).Warn("skipping invalid manifest ref")
						continue
					}

					storer, _ := storeFetchedManifests.SetKey(storeFetchedManifestsKey{valueID: av.GetValueID(), refIndex: i}, true)
					storer.value.SetResult(av.GetValue(), nil)
				}
			},
			// value removed
			func(av directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				for _, key := range storeFetchedManifests.GetKeys() {
					if key.valueID == av.GetValueID() {
						storeFetchedManifests.RemoveKey(key)
					}
				}
			},
			// disposed (ignore, only happens once ctx cancels)
			nil, // func() {},
			nil,
		)
	}

	_, ref, err := t.c.bus.AddDirective(
		bldr_manifest.NewFetchManifest(
			// use pluginID as manifest id
			t.pluginID,
			// accept any build type (presuming we only have the right one)
			nil,
			// accept any of the platform ids we have
			platformIDs,
			// accept any revision
			0,
		),
		handler,
	)
	if err != nil {
		return err
	}

	// we are done
	_ = context.AfterFunc(ctx, ref.Release)
	return nil
}

type fetchManifestValueStorer struct {
	pi     *pluginInstance
	value  *promise.Promise[*bldr_manifest.FetchManifestValue]
	refIdx int
}

func (t *pluginInstance) newManifestFetchValueStorer(key storeFetchedManifestsKey) (keyed.Routine, *fetchManifestValueStorer) {
	s := &fetchManifestValueStorer{pi: t, refIdx: key.refIndex}
	s.value = promise.NewPromise[*bldr_manifest.FetchManifestValue]()
	return s.execFetchManifestValueStorer, s
}

// execFetchManifestValueStorer executes storing the FetchManifest value in storage.
func (t *fetchManifestValueStorer) execFetchManifestValueStorer(ctx context.Context) error {
	fetchManifestValue, err := t.value.Await(ctx)
	if err != nil {
		return err
	}

	manifestRefs := fetchManifestValue.GetManifestRefs()
	if len(manifestRefs) <= t.refIdx {
		return nil
	}

	manifestRef := manifestRefs[t.refIdx]
	meta := manifestRef.GetMeta()
	le := meta.Logger(t.pi.le)
	le.Debug("downloading and storing plugin manifest ref")

	ws, err := t.pi.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	manifestKey := bldr_manifest.NewManifestKey(t.pi.c.objKey, meta)
	prevManifest, prevManifestFound, err := ws.GetObject(ctx, manifestKey)
	if err != nil {
		return err
	}
	var prevManifestRootRef *bucket.ObjectRef
	if prevManifest != nil {
		prevManifestRootRef, _, err = prevManifest.GetRootRef(ctx)
		if err != nil {
			return err
		}
	}

	// TODO: See manifest/builder/controller/controller.go
	// TODO: We don't increment the manifest revision in devtool mode when hot reloading.
	// TODO: Instead make sure the root ref is the same in storage here.
	if prevManifestFound && prevManifestRootRef.EqualVT(manifestRef.GetManifestRef()) {
		// manifest exists, do nothing.
		le.Debug("manifest is identical, skipping")
		return nil
	}

	// detects changes and does nothing if there are no changes
	le.
		WithFields(logrus.Fields{
			"manifest-ref": manifestRef.GetManifestRef().String(),
			"host-obj-key": t.pi.c.objKey,
		}).
		Debug("registering fetched plugin manifest ref")
	err = bldr_manifest_world.ExStoreManifestOp(
		ctx,
		ws,
		t.pi.c.peerID,
		manifestKey,
		[]string{t.pi.c.objKey},
		manifestRef,
	)
	if err != nil {
		return err
	}

	le.Info("successfully fetched and stored manifest ref")
	return nil
}
