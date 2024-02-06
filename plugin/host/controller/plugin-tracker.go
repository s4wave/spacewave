package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	backoff "github.com/aperturerobotics/util/backoff"
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
	backoffConf := t.c.conf.GetExecBackoff().CloneVT()
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
	t.c.rmtx.Lock()
	pluginManifest := t.c.pluginManifests[pluginID]
	manifest := pluginManifest.manifest
	hostMux := t.c.buildPluginMux(pluginID, pluginManifest, proxyHostVol, hostVol.info)
	t.c.rmtx.Unlock()

	// build world state handle
	ws, err := t.c.getWorldState(ctx)
	if err != nil {
		return err
	}

	// fetch the manifest if it doesn't exist in the cache
	emptyManifest := manifest.GetMeta().GetManifestId() == ""
	if emptyManifest || t.c.conf.GetAlwaysFetchManifest() {
		ref, fetcher, _ := t.c.pluginManifestFetchers.AddKeyRef(pluginID)
		defer ref.Release()

		if emptyManifest {
			// expect that we will be reset by the changing plugin manifest
			_, err := fetcher.resultPromise.Await(ctx)
			return err
		}
	}

	le.Infof("starting plugin with manifest: %s", pluginManifest.manifestRef.MarshalString())
	return manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, pluginManifest.manifestRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
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
