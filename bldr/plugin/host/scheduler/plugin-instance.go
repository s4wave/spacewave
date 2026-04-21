package plugin_host_scheduler

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// pluginInstance manages a running plugin instance
//
// downloadManifestRoutine: given a manifest from FetchManfest, downloads + stores in the world.
// watchWorldManifestRoutine: watches the world for the latest manifest for the plugin.
// executePluginRoutine: with a ManifestSnapshot from watchWorldManifestRoutine, executes the plugin.
type pluginInstance struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id
	pluginID string
	// instanceKey is the instance key (empty for shared instances).
	instanceKey string
	// loggedNotFound indicates if we logged no manifests were found
	loggedNotFound atomic.Bool

	// runningPluginCtr contains the running plugin ref
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]

	// fetchWorldManifestRoutine calls FetchManifest and stores the results to the world.
	fetchWorldManifestRoutine *routine.StateRoutineContainer[*pluginHostSet]

	// watchWorldManifestRoutine watches the world for the latest manifest for the plugin.
	watchWorldManifestRoutine *routine.StateRoutineContainer[*pluginHostSet]

	// downloadManifestRoutine is the routine to download the contents of a manifest to a local bucket
	// this routine only runs if watchWorldManifestRoutine triggers it.
	downloadManifestRoutine *routine.StateRoutineContainer[*bldr_manifest.ManifestSnapshot]
	// executePluginRoutine is the routine to execute a plugin with a manifest.
	executePluginRoutine *routine.StateRoutineContainer[*executePluginArgs]
}

// GetRunningPluginCtr returns the current running plugin instance.
// May be changed (or set to nil) when the instance changes.
func (t *pluginInstance) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return t.runningPluginCtr
}

// newPluginInstance constructs a new execute plugin routine.
// key is the composite key: pluginID or pluginID/instanceKey.
func (c *Controller) newPluginInstance(key string) (keyed.Routine, *pluginInstance) {
	pluginID, instanceKey, _ := strings.Cut(key, "/")
	le := c.le.WithField("plugin-id", pluginID)
	if instanceKey != "" {
		le = le.WithField("instance-key", instanceKey)
	}
	tr := &pluginInstance{
		c:                c,
		le:               le,
		pluginID:         pluginID,
		instanceKey:      instanceKey,
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}

	fetchBackoff, execBackoff := c.conf.BuildFetchBackoff(), c.conf.BuildExecBackoff()

	tr.fetchWorldManifestRoutine = routine.NewStateRoutineContainerWithLogger(pluginHostSetEqual, le, routine.WithRetry(fetchBackoff))
	tr.fetchWorldManifestRoutine.SetStateRoutine(tr.execFetchWorldManifest)

	tr.watchWorldManifestRoutine = routine.NewStateRoutineContainerWithLogger(pluginHostSetEqual, le, routine.WithRetry(fetchBackoff))
	tr.watchWorldManifestRoutine.SetStateRoutine(tr.execWatchWorldManifest)

	tr.downloadManifestRoutine = routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](
		le,
		routine.WithRetry(fetchBackoff),
		// TODO: Detect issues copying entrypoint manifests.
		/*
			routine.WithExitCb(func(err error) {
			}),
		*/
	)
	tr.downloadManifestRoutine.SetStateRoutine(tr.execDownloadManifest)

	tr.executePluginRoutine = routine.NewStateRoutineContainerWithLogger(
		executePluginArgsEqual,
		le,
		routine.WithRetry(execBackoff),
	)
	tr.executePluginRoutine.SetStateRoutine(tr.execPlugin)

	return tr.execute, tr
}

// execute executes the routine.
func (t *pluginInstance) execute(ctx context.Context) error {
	// Fetch manifests
	if t.c.conf.GetWatchFetchManifest() {
		t.fetchWorldManifestRoutine.SetContext(ctx, true)
		defer t.fetchWorldManifestRoutine.ClearContext()
	}

	// Watch the world state for the latest fully-downloaded manifest.
	t.watchWorldManifestRoutine.SetContext(ctx, true)
	defer t.watchWorldManifestRoutine.ClearContext()

	// Download manifests when the FetchManifest directive changes values.
	// Managed by the watchWorldManifestRoutine.
	t.downloadManifestRoutine.SetContext(ctx, true)
	defer t.downloadManifestRoutine.ClearContext()

	// Set the context for the execute plugin routine.
	t.executePluginRoutine.SetContext(ctx, true)
	defer t.executePluginRoutine.ClearContext()

	// Watch the set of plugin hosts.
	return ccontainer.WatchChanges(
		ctx,
		nil,
		t.c.pluginHostsCtr,
		func(msg *pluginHostSet) error {
			t.fetchWorldManifestRoutine.SetState(msg)
			t.watchWorldManifestRoutine.SetState(msg)
			return nil
		},
		nil,
	)
}

// _ is a type assertion
var _ bldr_plugin.RunningPluginRef = ((*pluginInstance)(nil))
