package plugin_host_scheduler

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
)

// executePluginArgs contains the arguments for executing a plugin.
type executePluginArgs struct {
	manifestSnapshot *bldr_manifest.ManifestSnapshot
	pluginHost       bldr_plugin_host.PluginHost
}

// executePluginArgsEqual compares two executePluginArgs for equality.
func executePluginArgsEqual(a, b *executePluginArgs) bool {
	if a == nil || b == nil {
		return a == b
	}

	// Compare manifest snapshots
	manifestEqual := (a.manifestSnapshot == nil) == (b.manifestSnapshot == nil)
	if manifestEqual && a.manifestSnapshot != nil {
		// Compare the manifest references for equality
		manifestEqual = a.manifestSnapshot.GetManifestRef().EqualVT(b.manifestSnapshot.GetManifestRef())
	}
	if !manifestEqual {
		return false
	}

	// Compare plugin hosts
	pluginHostEqual := (a.pluginHost == nil) == (b.pluginHost == nil)
	if pluginHostEqual && a.pluginHost != nil {
		pluginHostEqual = a.pluginHost == b.pluginHost
	}

	return pluginHostEqual
}

// execPlugin executes the plugin.
func (t *pluginInstance) execPlugin(ctx context.Context, args *executePluginArgs) error {
	if args == nil || args.manifestSnapshot == nil {
		return nil
	}
	pluginManifest := args.manifestSnapshot
	pluginID, le := t.pluginID, t.le

	// build proxy volume
	hostVol, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	proxyHostVol := volume_rpc_server.NewProxyVolume(ctx, hostVol.vol, false)

	// build world state handle
	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
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
		distFsID := bldr_plugin.PluginDistFsId(pluginID)
		distAccessCtrl := unixfs_access.NewControllerWithHandle(
			le,
			t.c.bus,
			&controller.Info{
				Id:          ControllerID + distFsID,
				Version:     Version.String(),
				Description: "plugin dist fs for plugin: " + pluginID,
			},
			[]string{distFsID},
			distFS,
		)
		defer distAccessCtrl.Close()

		// mount the dist fs access controller
		relDistAccessCtrl, err := t.c.bus.AddController(ctx, distAccessCtrl, nil)
		if err != nil {
			return err
		}
		defer relDistAccessCtrl()

		// expose the plugin assets as a unixfs on the host bus
		// this enables serving /b/pa/... requests
		assetsFsID := bldr_plugin.PluginAssetsFsId(pluginID)
		assetsAccessCtrl := unixfs_access.NewControllerWithHandle(
			le,
			t.c.bus,
			&controller.Info{
				Id:          ControllerID + assetsFsID,
				Version:     Version.String(),
				Description: "plugin assets fs for plugin: " + pluginID,
			},
			[]string{assetsFsID},
			assetsFS,
		)
		defer assetsAccessCtrl.Close()

		// mount the dist fs access controller
		relAssetsAccessCtrl, err := t.c.bus.AddController(ctx, assetsAccessCtrl, nil)
		if err != nil {
			return err
		}
		defer relAssetsAccessCtrl()

		// build the mux for handling incoming RPCs from the plugin
		hostMux, relHostMux := t.c.buildPluginMux(
			ctx,
			pluginID,
			pluginManifest,
			proxyHostVol,
			hostVol.info,
			distFS,
			assetsFS,
		)
		defer relHostMux()

		execErr := args.pluginHost.ExecutePlugin(
			ctx,
			pluginID,
			t.instanceKey,
			manifest.GetEntrypoint(),
			distFS,
			assetsFS,
			hostMux,
			func(client srpc.Client) error { t.updateRpcClient(client); return nil },
		)

		// clear the rpc client after the plugin exits
		t.updateRpcClient(nil)

		// handle if the plugin returned an error
		if execErr != nil {
			if ctx.Err() != nil {
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
func (t *pluginInstance) updateRpcClient(client srpc.Client) {
	_ = t.runningPluginCtr.SwapValue(func(rp bldr_plugin.RunningPlugin) bldr_plugin.RunningPlugin {
		var val srpc.Client
		if rp != nil {
			val = rp.GetRpcClient()
		}
		changed := val != client
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
