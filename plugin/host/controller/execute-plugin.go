package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/sirupsen/logrus"
)

// executePlugin manages a running plugin instance
//
// downloadManifestRoutine: given a manifest from FetchManfest, downloads + stores in the world.
//
// watchWorldManifestRoutine: watches the world for the latest manifest for the plugin.
// executePluginRoutine: with a ManifestSnapshot from watchWorldManifestRoutine, executes the plugin.
type executePlugin struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id
	pluginID string
	// runningPluginCtr contains the running plugin ref
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]

	// downloadManifestRoutine is the routine to download a manifest and store it in the world.
	// this routine only runs if watchFetchManifestRoutine triggers it.
	downloadManifestRoutine *routine.StateRoutineContainer[*bldr_manifest.FetchManifestValue]

	// watchWorldManifestRoutine watches the world for the latest manifest for the plugin.
	// updates executePluginRoutine state
	watchWorldManifestRoutine *routine.RoutineContainer
	// executePluginRoutine is the routine to execute a plugin with a manifest.
	executePluginRoutine *routine.StateRoutineContainer[*bldr_manifest.ManifestSnapshot]
}

// GetRunningPluginCtr returns the current running plugin instance.
// May be changed (or set to nil) when the instance changes.
func (t *executePlugin) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return t.runningPluginCtr
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *executePlugin) {
	le := c.le.WithField("plugin-id", key)
	tr := &executePlugin{
		c:                c,
		le:               le,
		pluginID:         key,
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}

	fetchBackoff, execBackoff := c.conf.BuildFetchBackoff(), c.conf.BuildExecBackoff()

	tr.watchWorldManifestRoutine = routine.NewRoutineContainerWithLogger(le, routine.WithRetry(fetchBackoff))
	tr.watchWorldManifestRoutine.SetRoutine(tr.execWatchWorldManifest)

	tr.downloadManifestRoutine = routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.FetchManifestValue](le, routine.WithRetry(fetchBackoff))
	tr.downloadManifestRoutine.SetStateRoutine(tr.execDownloadManifest)

	tr.executePluginRoutine = routine.NewStateRoutineContainerWithLogger(
		func(v1, v2 *bldr_manifest.ManifestSnapshot) bool {
			// Ignore the manifest rev, just compare the root ref.
			//
			// This is to avoid an unnecessary refresh when we overwrite the
			// entrypoint manifest w/ rev 0 with the launcher-provided manifest.
			return v1.GetManifest().GetAssetsFsRef().EqualVT(v2.GetManifest().GetAssetsFsRef()) &&
				v1.GetManifest().GetDistFsRef().EqualVT(v2.GetManifest().GetDistFsRef()) &&
				v1.GetManifest().GetEntrypoint() == v2.GetManifest().GetEntrypoint()
		},
		le,
		routine.WithRetry(execBackoff),
	)
	tr.executePluginRoutine.SetStateRoutine(tr.execPlugin)

	return tr.execute, tr
}

// execute executes the routine.
func (t *executePlugin) execute(ctx context.Context) error {
	// Keep the FetchManifest directive running if requested
	le := t.le
	if t.c.conf.GetWatchFetchManifest() {
		// determine host plugin platform id
		hostPluginPlatformID, err := t.c.hostPluginPlatformID.Await(ctx)
		if err != nil {
			return err
		}

		fetchMeta := &bldr_manifest.ManifestMeta{
			ManifestId: t.pluginID,
			PlatformId: hostPluginPlatformID,
		}
		fetchMeta.Logger(le).Debug("starting to watch FetchManifest")
		_, fetchManifestRef, err := bldr_manifest.WatchLatestManifestValue(
			t.c.bus,
			fetchMeta,
			func(tval directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
				if tval == nil || tval.GetValue() == nil {
					le.Debug("FetchManifest directive has no value currently")
					t.downloadManifestRoutine.SetState(nil)
					return
				}

				// Manifest has changed, trigger a restart of downloadManifest
				val := tval.GetValue()
				le.Debugf("FetchManifest returned manifest with rev %v", val.GetManifestRef().GetMeta().GetRev())
				t.downloadManifestRoutine.SetState(val)
			},
		)
		if err != nil {
			// we should be able to create this directive
			return err
		}
		defer fetchManifestRef.Release()
	}

	// Download manifests when the FetchManifest directive above changes values.
	// This compares the downloaded manifest with the one in storage and keeps the higher rev.
	t.downloadManifestRoutine.SetContext(ctx, true)
	defer t.downloadManifestRoutine.ClearContext()

	// Watch the world state for the latest fully-downloaded manifest.
	t.watchWorldManifestRoutine.SetContext(ctx, true)
	defer t.watchWorldManifestRoutine.ClearContext()

	// Set the context for the execute plugin routine.
	t.executePluginRoutine.SetContext(ctx, true)
	defer t.executePluginRoutine.ClearContext()

	// TODO set manifest on StateRoutine
	<-ctx.Done()
	return nil
}

// execPlugin executes the plugin.
func (t *executePlugin) execPlugin(ctx context.Context, pluginManifest *bldr_manifest.ManifestSnapshot) error {
	pluginID, le := t.pluginID, t.le

	// build proxy volume
	hostVol, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	proxyHostVol := volume_rpc_server.NewProxyVolume(ctx, hostVol.vol, false)

	// build world state handle
	ws, err := t.c.getWorldState(ctx)
	if err != nil {
		return err
	}

	le.Infof("starting plugin with manifest: %s", pluginManifest.GetManifestRef().MarshalString())
	return manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, pluginManifest.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS,
		assetsFS *unixfs.FSHandle,
	) error {
		// expose the plugin dist as a unixfs on the host bus
		// this enables serving /b/pd/... requests
		distFsID := bldr_plugin.PluginDistFsId + "/" + pluginID
		distAccessCtrl := unixfs_access.NewControllerWithHandle(
			le,
			t.c.bus,
			&controller.Info{
				Id:          t.c.info.GetId() + distFsID,
				Version:     t.c.info.GetVersion(),
				Description: "plugin dist fs for plugin: " + pluginID,
			},
			distFsID,
			distFS,
		)
		defer distAccessCtrl.Close()

		// mount the dist fs access controller
		relDistAccessCtrl, err := t.c.bus.AddController(ctx, distAccessCtrl, nil)
		if err != nil {
			return err
		}
		defer relDistAccessCtrl()

		// build the mux for handling incoming RPCs from the plugin
		hostMux := t.c.buildPluginMux(
			pluginID,
			pluginManifest,
			proxyHostVol,
			hostVol.info,
			distFS,
			assetsFS,
		)

		// execute the plugin
		execErr := t.c.host.ExecutePlugin(
			ctx,
			pluginID,
			manifest.GetEntrypoint(),
			distFS,
			hostMux,
			func(client srpc.Client) error { t.updateRpcClient(client); return nil },
		)

		// clear the rpc client after the plugin exits
		t.updateRpcClient(nil)

		// handle if the plugin returned an error
		if execErr != nil {
			select {
			case <-ctx.Done():
				// if the context was canceled, return that error instead.
				return context.Canceled
			default:
			}
			// TODO: track this error in PluginStatus
			le.WithError(execErr).Error("plugin execution errored")
			return execErr
		}

		return nil
	})
}

// updateRpcClient is called by the plugin when the RPC client changes.
func (t *executePlugin) updateRpcClient(client srpc.Client) {
	_ = t.runningPluginCtr.SwapValue(func(rp bldr_plugin.RunningPlugin) bldr_plugin.RunningPlugin {
		var val srpc.Client
		if rp != nil {
			val = rp.GetRpcClient()
		}
		changed := ((client == nil) != (val == nil)) || (val != nil && val != client)
		if !changed {
			return rp
		}
		if client == nil {
			t.le.Debug("plugin rpc client is unset")
			return nil
		}
		t.le.Debug("plugin rpc client is ready")
		return bldr_plugin.NewRunningPlugin(client)
	})
}

// _ is a type assertion
var _ bldr_plugin.RunningPluginRef = ((*executePlugin)(nil))
