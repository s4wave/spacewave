package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
)

// watchFetchManifest tracks watching FetchManifest directives.
type watchFetchManifest struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
}

// newWatchFetchManifest constructs a new FetchManifest watcher routine.
func (c *Controller) newWatchFetchManifest(pluginID string) (keyed.Routine, *watchFetchManifest) {
	wf := &watchFetchManifest{
		c:        c,
		pluginID: pluginID,
	}
	return wf.execute, wf
}

// execute executes the FetchManifest watcher.
func (w *watchFetchManifest) execute(ctx context.Context) error {
	// determine host plugin platform id
	hostPluginPlatformID, err := w.c.hostPluginPlatformID.Await(ctx)
	if err != nil {
		return err
	}

	// register FetchManifest directive
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: w.pluginID,
		PlatformId: hostPluginPlatformID,
	}

	// Add FetchManifest directive
	_, fetchRef, err := w.c.bus.AddDirective(bldr_manifest.NewFetchManifest(meta), nil)
	if err != nil {
		return err
	}
	defer fetchRef.Release()

	var prevResult *bldr_manifest.FetchManifestValue
	le := w.c.le.WithField("plugin-id", w.pluginID)
	le.Debug("starting to watch FetchManifest")
	return bus.ExecOneOffWatchLatestCb(
		ctx,
		w.c.bus,
		bldr_manifest.NewFetchManifest(meta),
		nil,
		func(tval directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) error {
			if tval == nil || tval.GetValue() == nil {
				le.Debug("FetchManifest directive has no value currently")
				return nil
			}

			// Manifest has changed, trigger a restart of downloadManifest
			val := tval.GetValue()
			le.Debugf("FetchManifest returned manifest with rev %v", val.GetManifestRef().GetMeta().GetRev())
			if prevResult != nil && !val.EqualVT(prevResult) && !w.c.conf.GetDisableStoreManifest() {
				le.Debug("manifest changed, triggering fetcher restart")
				if _, reset := w.c.downloadManifests.RestartRoutine(w.pluginID); reset {
					le.Info("restarted outdated plugin fetcher and instance")
				}
			}

			prevResult = val
			return nil
		},
	)
}
