package plugin_host_scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	"github.com/sirupsen/logrus"
)

// pluginInstance manages a running plugin instance
//
// downloadManifestRoutine: given a manifest from FetchManifest, downloads + stores in the world.
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
	// bindingKey is the logical installation, without a physical generation suffix.
	bindingKey string
	// manifestRoot restricts selection to the requested executable.
	manifestRoot string
	// start opens only after the first reference installs its selection policy.
	start *ccontainer.CContainer[bool]
	// selectedManifest is the explicit installation target, independent of catalog order.
	selectedManifest atomic.Pointer[installedManifests]
	// selections and selectionSequence track retained installation demands under pluginUpdateMtx.
	selections        map[uint64]*installedManifests
	selectionSequence uint64
	// physical marks an isolated worker whose parent publishes public status.
	physical bool
	// prepared delays this worker's registration admission until explicit activation.
	prepared bool
	// onLoadState projects an admitted worker's state into its logical binding.
	onLoadState func(bldr_plugin.PluginLoadState)
	// executions owns worker lifetimes independently of candidate-selection retries.
	executions *keyed.KeyedRefCount[executionReference, *pluginInstance]
	// activeExecution is the admitted worker, protected by pluginUpdateMtx.
	activeExecution *pluginInstance
	// releaseExecution retains the admitted worker through candidate changes.
	releaseExecution func()
	// executionsClosed prevents a finishing candidate from publishing after shutdown.
	executionsClosed bool
	// loggedNotFound indicates if we logged no manifests were found
	loggedNotFound atomic.Bool
	// manifestCopyAccounting owns demand and copy counters for the selected candidate.
	manifestCopyAccounting atomic.Pointer[manifestCopyAccounting]
	// manifestSelectionFingerprint is the last input set fully processed by
	// watchWorldManifestRoutine.
	manifestSelectionFingerprint atomic.Pointer[manifestSelectionInput]

	// startupWaitBudgetTimer holds the armed startup wait budget deadline.
	startupWaitBudgetTimer atomic.Pointer[time.Timer]

	// runningPluginCtr contains the running plugin ref
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]
	// pluginLoadStateCtr atomically tracks the RPC client and initial
	// capability-registration state.
	pluginLoadStateCtr *ccontainer.CContainer[bldr_plugin.PluginLoadState]

	// distAccess owns the lifetime of the plugin dist fs access provider
	distAccess *unixfs_access.RotatingAccess
	// assetsAccess owns the lifetime of the plugin assets fs access provider
	assetsAccess *unixfs_access.RotatingAccess

	// fetchWorldManifestRoutine calls FetchManifest and stores the results to the world.
	fetchWorldManifestRoutine *routine.StateRoutineContainer[*pluginHostSet]

	// watchWorldManifestRoutine watches the world for the latest manifest for the plugin.
	watchWorldManifestRoutine *routine.StateRoutineContainer[*pluginHostSet]

	// downloadManifestRoutine is the routine to download the contents of a manifest to a local bucket
	// this routine only runs if watchWorldManifestRoutine triggers it.
	downloadManifestRoutine *routine.StateRoutineContainer[*bldr_manifest.ManifestSnapshot]
	// manifestCopyStatus exposes this instance's copy class for tests and diagnostics.
	manifestCopyStatus *ccontainer.CContainer[*manifestCopyStatus]
	// pluginUpdateMtx serializes candidate selection and guarded replacement.
	pluginUpdateMtx sync.Mutex
	// updatePluginRoutine waits for the running plugin to permit replacement.
	updatePluginRoutine *routine.StateRoutineContainer[*executePluginArgs]
	// executePluginRoutine is the routine to execute a plugin with a manifest.
	executePluginRoutine *routine.StateRoutineContainer[*executePluginArgs]
}

// GetRunningPluginCtr returns the current running plugin instance.
// May be changed (or set to nil) when the instance changes.
func (t *pluginInstance) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return t.runningPluginCtr
}

// GetPluginLoadStateCtr returns the atomic RPC-client and initial
// capability-registration state for the plugin instance.
func (t *pluginInstance) GetPluginLoadStateCtr() ccontainer.Watchable[bldr_plugin.PluginLoadState] {
	return t.pluginLoadStateCtr
}

// newPluginInstance constructs a new execute plugin routine.
// key is the composite key: pluginID or pluginID/instanceKey.
func (c *Controller) newPluginInstance(key pluginReference) (keyed.Routine, *pluginInstance) {
	pluginID, instanceKey := key.pluginID, key.executionKey()
	le := c.le.WithField("plugin-id", pluginID)
	if instanceKey != "" {
		le = le.WithField("instance-key", instanceKey)
	}
	tr := newPluginState(c, le, pluginID, instanceKey, key.manifestRoot)
	tr.bindingKey = key.instanceKey
	if tr.bindingKey == "" {
		tr.bindingKey = c.conf.GetInstanceKey()
	}
	tr.executions = keyed.NewKeyedRefCountWithLogger(tr.newExecution, le)

	fetchBackoff, execBackoff := c.conf.BuildFetchBackoff(), c.conf.BuildExecBackoff()

	tr.fetchWorldManifestRoutine = routine.NewStateRoutineContainerWithLogger(pluginHostSetEqual, le, routine.WithRetry(fetchBackoff))
	tr.fetchWorldManifestRoutine.SetStateRoutine(tr.execFetchWorldManifest)

	tr.watchWorldManifestRoutine = routine.NewStateRoutineContainerWithLogger(pluginHostSetEqual, le, routine.WithRetry(fetchBackoff))
	tr.watchWorldManifestRoutine.SetStateRoutine(tr.execWatchWorldManifest)

	tr.downloadManifestRoutine = routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](
		le,
		routine.WithRetry(fetchBackoff),
	)
	tr.downloadManifestRoutine.SetStateRoutine(tr.execDownloadManifest)

	tr.executePluginRoutine = routine.NewStateRoutineContainerWithLogger(
		executePluginArgsEqual,
		le,
		routine.WithRetry(execBackoff),
	)
	tr.executePluginRoutine.SetStateRoutine(tr.execSelectedPlugin)

	tr.updatePluginRoutine = routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le, routine.WithRetry(fetchBackoff))
	tr.updatePluginRoutine.SetStateRoutine(tr.execGuardedPluginUpdate)

	return tr.execute, tr
}

// newPluginState constructs the state shared by logical bindings and isolated workers.
func newPluginState(c *Controller, le *logrus.Entry, pluginID, instanceKey, manifestRoot string) *pluginInstance {
	return &pluginInstance{
		c:                c,
		le:               le,
		pluginID:         pluginID,
		instanceKey:      instanceKey,
		manifestRoot:     manifestRoot,
		start:            ccontainer.NewCContainer(false),
		selections:       make(map[uint64]*installedManifests),
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer[bldr_plugin.PluginLoadState](
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
		manifestCopyStatus: ccontainer.NewCContainer[*manifestCopyStatus](nil),
		distAccess:         unixfs_access.NewRotatingAccess(),
		assetsAccess:       unixfs_access.NewRotatingAccess(),
	}
}

// execute executes the routine.
func (t *pluginInstance) execute(ctx context.Context) error {
	if _, err := t.start.WaitValue(ctx, nil); err != nil {
		return err
	}
	if err := t.c.ensureManifestStore(ctx); err != nil {
		return err
	}

	startupWaitBudget, err := t.c.conf.BuildStartupWaitBudget()
	if err != nil {
		return err
	}
	t.armStartupWaitBudget(startupWaitBudget)
	defer t.stopStartupWaitBudget()

	t.c.setPluginStatus(
		t.pluginID,
		t.instanceKey,
		bldr_plugin.PluginState_PluginState_REQUESTED,
	)
	defer t.c.setPluginStatus(
		t.pluginID,
		t.instanceKey,
		bldr_plugin.PluginState_PluginState_UNKNOWN,
	)

	// Fetch manifests
	if t.manifestRoot == "" && t.c.conf.GetWatchFetchManifest() {
		t.fetchWorldManifestRoutine.SetContext(ctx, true)
		defer t.fetchWorldManifestRoutine.ClearContext()
	}

	// Direct fetch owns the current target in no-store mode. Retained replay
	// artifacts must not compete with that target after a compiler update.
	// Exact historical requests still resolve through the stored manifest graph.
	if t.manifestRoot != "" || !t.c.conf.GetWatchFetchManifest() || !t.c.conf.GetDisableStoreManifest() {
		t.watchWorldManifestRoutine.SetContext(ctx, true)
		defer t.watchWorldManifestRoutine.ClearContext()
	}

	// Download manifests when the FetchManifest directive changes values.
	// Managed by the watchWorldManifestRoutine.
	t.downloadManifestRoutine.SetContext(ctx, true)
	defer t.downloadManifestRoutine.ClearContext()

	// Workers outlive a candidate-selection attempt. Release the admitted worker
	// only when replaced, explicitly removed, or this logical binding closes.
	t.pluginUpdateMtx.Lock()
	t.executionsClosed = false
	t.pluginUpdateMtx.Unlock()
	t.executions.SetContext(ctx, true)
	defer t.executions.ClearContext()
	defer t.closeExecutions()

	// Set the context for the execute plugin routine.
	t.executePluginRoutine.SetContext(ctx, true)
	defer t.executePluginRoutine.ClearContext()

	t.updatePluginRoutine.SetContext(ctx, true)
	defer t.updatePluginRoutine.ClearContext()

	distFsID := bldr_plugin.PluginDistFsId(t.pluginID)
	if t.manifestRoot != "" {
		distFsID += "/manifest/" + t.manifestRoot
	}
	distAccessCtrl := unixfs_access.NewController(
		t.le,
		t.c.bus,
		&controller.Info{
			Id:          ControllerID + distFsID,
			Version:     Version.String(),
			Description: "plugin dist fs for plugin: " + t.pluginID,
		},
		[]string{distFsID},
		t.distAccess.AccessUnixFS,
	)
	defer distAccessCtrl.Close()

	relDistAccessCtrl, err := t.c.bus.AddController(ctx, distAccessCtrl, nil)
	if err != nil {
		return err
	}
	defer relDistAccessCtrl()

	assetsFsID := bldr_plugin.PluginAssetsFsId(t.pluginID)
	if t.manifestRoot != "" {
		assetsFsID += "/manifest/" + t.manifestRoot
	}
	assetsAccessCtrl := unixfs_access.NewController(
		t.le,
		t.c.bus,
		&controller.Info{
			Id:          ControllerID + assetsFsID,
			Version:     Version.String(),
			Description: "plugin assets fs for plugin: " + t.pluginID,
		},
		[]string{assetsFsID},
		t.assetsAccess.AccessUnixFS,
	)
	defer assetsAccessCtrl.Close()

	relAssetsAccessCtrl, err := t.c.bus.AddController(ctx, assetsAccessCtrl, nil)
	if err != nil {
		return err
	}
	defer relAssetsAccessCtrl()

	// Watch the set of plugin hosts.
	return ccontainer.WatchChanges(
		ctx,
		nil,
		t.c.pluginHostsCtr,
		func(msg *pluginHostSet) error {
			t.fetchWorldManifestRoutine.SetState(msg)
			t.watchWorldManifestRoutine.SetState(msg)
			t.pluginUpdateMtx.Lock()
			t.selectInstalledManifestLocked(msg)
			t.pluginUpdateMtx.Unlock()
			return nil
		},
		nil,
	)
}

// ensureAccessProviders initializes the dist and assets access providers
// if unset. Some construction paths (e.g. tests) build partial instances.
func (t *pluginInstance) ensureAccessProviders() {
	if t.distAccess == nil {
		t.distAccess = unixfs_access.NewRotatingAccess()
	}
	if t.assetsAccess == nil {
		t.assetsAccess = unixfs_access.NewRotatingAccess()
	}
}

// _ is a type assertion
var _ bldr_plugin.RunningPluginRef = (*pluginInstance)(nil)
