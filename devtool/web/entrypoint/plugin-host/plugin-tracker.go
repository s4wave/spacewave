package devtool_web_entrypoint_plugin_host

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/retry"
	"github.com/sirupsen/logrus"
)

// pluginTracker manages a running plugin instance
type pluginTracker struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id
	pluginID string
	// runningPluginCtr contains the running plugin ref
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]
}

// GetRunningPluginCtr returns the current running plugin instance.
// May be changed (or set to nil) when the instance changes.
func (t *pluginTracker) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return t.runningPluginCtr
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *pluginTracker) {
	tr := &pluginTracker{
		c:                c,
		le:               c.le.WithField("plugin-id", key),
		pluginID:         key,
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}
	return tr.execute, tr
}

// execute executes the routine.
func (t *pluginTracker) execute(ctx context.Context) error {
	bo := t.c.conf.GetExecBackoff().Construct()

	return retry.Retry(
		ctx,
		t.c.le.WithField("plugin-id", t.pluginID),
		func(ctx context.Context, success func()) error {
			err := t.execPlugin(ctx)
			if err == nil {
				success()
			}
			return err
		},
		bo,
	)
}

// execPlugin executes the plugin.
func (t *pluginTracker) execPlugin(ctx context.Context) error {
	pluginID, le := t.pluginID, t.le

	// build proxy volume
	hostVol, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	proxyHostVol := volume_rpc_server.NewProxyVolume(ctx, hostVol.vol, false)

	// build mux
	t.c.mtx.Lock()
	pluginManifestSnapshot := t.c.pluginManifests[pluginID]
	manifestRef := pluginManifestSnapshot.GetManifestRef()
	manifest := pluginManifestSnapshot.GetManifest()
	hostMux := t.c.buildPluginMux(pluginID, pluginManifestSnapshot, proxyHostVol, hostVol.info)
	t.c.mtx.Unlock()

	// fetch the manifest
	// this is a separate tracker so it can restart this tracker if the manifest changes.
	fetcher, _ := t.c.pluginManifestTrackers.SetKey(pluginID, true)
	defer t.c.pluginManifestTrackers.RemoveKey(pluginID)

	// if the manifest does not exist in the cache, wait for the fetcher.
	emptyManifest := manifest.GetMeta().GetManifestId() == ""
	if emptyManifest {
		t.c.le.
			WithField("plugin-id", pluginID).
			Info("waiting for plugin manifest")
		// expect that we will be reset by the changing plugin manifest
		_, err := fetcher.resultPromise.Await(ctx)
		return err
	}

	t.c.le.
		WithField("plugin-id", pluginID).
		Infof("got plugin manifest: %v", manifest.String())
	bcs, err := bucket_lookup.BuildCursor(
		ctx,
		t.c.bus,
		t.c.le,
		t.c.sfs,
		hostVol.info.GetVolumeId(),
		manifestRef,
		nil,
	)
	if err != nil {
		return err
	}
	defer bcs.Release()

	le.Infof("starting plugin with manifest: %s", manifestRef.MarshalString())
	return bldr_manifest.AccessManifest(
		ctx,
		le,
		bcs,
		func(
			ctx context.Context,
			bls *bucket_lookup.Cursor,
			bcs *block.Cursor,
			manifest *bldr_manifest.Manifest,
			distFS *unixfs.FSHandle,
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
				if ctx.Err() != nil {
					// if the context was canceled, return that error instead.
					return context.Canceled
				}
				// TODO: track this error in PluginStatus
				le.WithError(execErr).Error("plugin execution errored")
				return execErr
			}

			return nil
		})
}

// updateRpcClient is called by the plugin when the RPC client changes.
func (t *pluginTracker) updateRpcClient(client srpc.Client) {
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
var _ bldr_plugin.RunningPluginRef = ((*pluginTracker)(nil))
