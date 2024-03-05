package devtool_web_entrypoint_plugin_host

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/directive"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// pluginManifestTracker tracks matched PluginManifest objects.
// updates the in-memory cache & restarts plugin if the manifest changes.
type pluginManifestTracker struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// resultPromise contains the result of the fetcher
	resultPromise *promise.PromiseContainer[*bldr_manifest.ManifestSnapshot]
}

// newPluginManifestTracker constructs a new plugin manifest tracker routine.
func (c *Controller) newPluginManifestTracker(key string) (keyed.Routine, *pluginManifestTracker) {
	tr := &pluginManifestTracker{
		c:             c,
		pluginID:      key,
		resultPromise: promise.NewPromiseContainer[*bldr_manifest.ManifestSnapshot](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginManifestTracker) execute(ctx context.Context) error {
	platformID, err := t.c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return err
	}

	hostVolInfo, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	di, diRef, err := bldr_manifest.FetchLatestManifestEffect(
		ctx,
		t.c.bus,
		&bldr_manifest.ManifestMeta{
			ManifestId: t.pluginID,
			PlatformId: platformID,
		},
		func(ctx context.Context, val *bldr_manifest.FetchManifestValue) (*bucket_lookup.Cursor, error) {
			return bucket_lookup.BuildCursor(
				ctx,
				t.c.bus,
				t.c.le,
				t.c.sfs,
				hostVolInfo.info.GetVolumeId(),
				val.GetManifestRef().GetManifestRef(),
				nil,
			)
		},
		func(val directive.TransformedAttachedValue[*bldr_manifest.FetchManifestValue, *bldr_manifest.ManifestSnapshot]) func() {
			manifestSnapshot := val.GetTransformedValue()
			t.resultPromise.SetResult(manifestSnapshot, nil)

			// update the manifest in the set
			t.c.mtx.Lock()
			existing := t.c.pluginManifests[t.pluginID]
			changed := !manifestSnapshot.EqualVT(existing)
			if changed {
				t.c.le.
					WithField("plugin-id", t.pluginID).
					Infof(
						"plugin manifest updated: %s => %s",
						manifestSnapshot.GetManifest().GetMeta().GetPlatformId(),
						manifestSnapshot.GetManifestRef().MarshalB58(),
					)
				t.c.pluginManifests[t.pluginID] = manifestSnapshot

				// restart the plugin, if running
				if _, reset := t.c.pluginInstances.RestartRoutine(t.pluginID); reset {
					t.c.le.
						WithField("plugin-id", t.pluginID).
						Info("restarted outdated plugin instance")
				}
			}
			t.c.mtx.Unlock()

			return func() {
				t.resultPromise.SetPromise(nil)
			}
		},
		keyed.WithExitLogger[uint32, directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]](t.c.le),
	)
	if err != nil {
		t.resultPromise.SetResult(nil, err)
		return err
	}
	defer diRef.Release()

	defer di.AddIdleCallback(func(errs []error) {
		for _, err := range errs {
			if err != context.Canceled && err != nil {
				handleErr(err)
				break
			}
		}
	})()

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		t.resultPromise.SetResult(nil, err)
		return err
	}
}
