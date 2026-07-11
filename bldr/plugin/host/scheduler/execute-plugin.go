package plugin_host_scheduler

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
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
		manifestEqual = manifest_world.ManifestObjectRefsSameExecutable(
			a.manifestSnapshot.GetManifestRef(),
			b.manifestSnapshot.GetManifestRef(),
		)
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
func (t *pluginInstance) execPlugin(ctx context.Context, args *executePluginArgs) (rerr error) {
	if args == nil ||
		args.manifestSnapshot == nil ||
		args.manifestSnapshot.GetManifestRef() == nil ||
		args.pluginHost == nil {
		return nil
	}
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/execute-plugin")
	defer task.End()
	t.ensureAccessProviders()
	defer func() {
		if rerr != nil {
			t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "execute plugin", rerr)
			return
		}
		t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
	}()

	pluginManifest := args.manifestSnapshot
	pluginID, le := t.pluginID, t.le
	trace.Log(ctx, "plugin-id", pluginID)
	trace.Log(ctx, "instance-key", t.instanceKey)
	trace.Log(ctx, "manifest-ref", pluginManifest.GetManifestRef().MarshalString())
	trace.Log(ctx, "startup-fetch-kind", "demand-plugin-execute")

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
		t.distAccess.SetCurrent(unixfs_access.NewAccessUnixFSFunc(distFS))
		defer t.distAccess.SetBlocked()
		t.assetsAccess.SetCurrent(unixfs_access.NewAccessUnixFSFunc(assetsFS))
		defer t.assetsAccess.SetBlocked()

		t.emitPluginManifestRoot(pluginManifest.GetManifestRef().GetRootRef().GetHash().MarshalString())

		hostRoot, _, hostRootRef, err := plugin_host_root.ExLookupRootByPlatform(
			ctx,
			t.c.bus,
			false,
			args.pluginHost.GetPlatformId(),
			nil,
		)
		if err != nil {
			return err
		}
		defer hostRootRef.Release()

		// build the mux for handling incoming RPCs from the plugin
		hostMux, relHostMux := t.c.buildPluginMux(
			ctx,
			pluginID,
			pluginManifest,
			proxyHostVol,
			hostVol.info,
			distFS,
			assetsFS,
			hostRoot,
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
			t.c.setPluginStatus(
				t.pluginID,
				t.instanceKey,
				bldr_plugin.PluginState_PluginState_REQUESTED,
			)
			return nil
		}
		t.le.Debug("plugin rpc client is ready")
		t.c.setPluginStatusClearingError(
			t.pluginID,
			t.instanceKey,
			bldr_plugin.PluginState_PluginState_RUNNING,
		)
		return bldr_plugin.NewRunningPlugin(client)
	})
}
