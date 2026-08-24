package plugin_host_scheduler

import (
	"context"
	"sync"

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

	manifestEqual := (a.manifestSnapshot == nil) == (b.manifestSnapshot == nil)
	if manifestEqual && a.manifestSnapshot != nil {
		manifestEqual = manifest_world.ManifestObjectRefsSameExecutable(
			a.manifestSnapshot.GetManifestRef(),
			b.manifestSnapshot.GetManifestRef(),
		)
	}
	if !manifestEqual {
		return false
	}

	pluginHostEqual := (a.pluginHost == nil) == (b.pluginHost == nil)
	if pluginHostEqual && a.pluginHost != nil {
		pluginHostEqual = a.pluginHost == b.pluginHost
	}

	return pluginHostEqual
}

// execPlugin executes the plugin.
// execPlugin executes the plugin with the given manifest snapshot on a
// plugin host.
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
			trace.Log(ctx, "manifest-copy-phase", "error")
			t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "execute plugin", rerr)
			return
		}
		t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
	}()
	pluginManifest := args.manifestSnapshot
	pluginID, le := t.pluginID, t.le
	accounting := t.manifestCopyAccountingForExecution(ctx, pluginManifest)
	accessCtx := ctx
	var demandObservation *manifestDemandObservation
	var finishDemand func(string)
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
	if accounting != nil {
		var readCounter *block.ReadCounter
		accessCtx, readCounter = block.WithReadCounter(accessCtx)
		demandObservation = &manifestDemandObservation{
			accounting: accounting,
			counter:    readCounter,
		}
		demandObservation.register()
		var demandTask *trace.Task
		accessCtx, demandTask = trace.NewTask(accessCtx, "bldr/plugin-host-scheduler/first-demand-block")
		trace.Log(accessCtx, "demand-read-phase", "waiting")
		var demandTaskOnce sync.Once
		finishDemand = func(phase string) {
			demandTaskOnce.Do(func() {
				trace.Log(accessCtx, "demand-read-phase", phase)
				demandTask.End()
			})
		}
	}
	defer func() {
		if demandObservation == nil {
			return
		}
		demandObservation.snapshot()
		if ctx.Err() != nil {
			finishDemand("canceled")
		} else {
			finishDemand("error")
		}
		demandObservation.finish()
	}()

	le.Infof("starting plugin with manifest: %s", pluginManifest.GetManifestRef().MarshalString())
	accessErr := manifest_world.AccessManifest(accessCtx, le, ws.AccessWorldState, pluginManifest.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS,
		assetsFS *unixfs.FSHandle,
	) error {
		if demandObservation != nil {
			snapshot := demandObservation.snapshot()
			if snapshot.BlockReadCount != 0 {
				finishDemand("first-demand-block")
			} else {
				finishDemand("access-manifest-ready")
			}
		}
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

		t.beginInitialCapabilityRegistration()

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
			t.finishInitialCapabilityRegistration,
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
	if demandObservation != nil {
		demandObservation.snapshot()
		if accessErr != nil {
			finishDemand("error")
		} else {
			finishDemand("access-manifest-complete")
		}
	}
	return accessErr
}

// beginInitialCapabilityRegistration resets readiness for a new plugin
// instance execution.
func (t *pluginInstance) beginInitialCapabilityRegistration() {
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		next := bldr_plugin.NewPluginLoadState(
			nil,
			bldr_plugin.InitialCapabilityRegistrationPending,
		)
		if current.GetStartupBudgetExhausted() {
			next = next.WithStartupBudgetExhausted()
		}
		return next
	})
}

// updateRpcClient is called by the plugin when the RPC client changes.
func (t *pluginInstance) updateRpcClient(client srpc.Client) {
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		registrationState := current.GetInitialCapabilityRegistrationState()
		if client == nil {
			registrationState = bldr_plugin.InitialCapabilityRegistrationFailed
		}
		next := bldr_plugin.NewPluginLoadState(client, registrationState)
		if current.GetStartupBudgetExhausted() {
			next = next.WithStartupBudgetExhausted()
		}
		return next
	})
}

func (t *pluginInstance) finishInitialCapabilityRegistration(complete bool) {
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		registrationState := bldr_plugin.InitialCapabilityRegistrationFailed
		if complete {
			registrationState = bldr_plugin.InitialCapabilityRegistrationComplete
		}
		next := bldr_plugin.NewPluginLoadState(current.GetRpcClient(), registrationState)
		if current.GetStartupBudgetExhausted() {
			next = next.WithStartupBudgetExhausted()
		}
		return next
	})
	if complete {
		t.stopStartupWaitBudget()
	}
}

// updatePluginLoadState applies cb to the load state and refreshes the
// running-plugin projection inside the load state container's critical
// section, so concurrent updates cannot publish a stale projection.
func (t *pluginInstance) updatePluginLoadState(
	cb func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState,
) bldr_plugin.PluginLoadState {
	return t.pluginLoadStateCtr.SwapValue(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		next := cb(current)
		t.publishPluginLoadState(next)
		return next
	})
}

func (t *pluginInstance) publishPluginLoadState(state bldr_plugin.PluginLoadState) {
	running := state.GetRunningPlugin()
	t.runningPluginCtr.SetValue(running)
	if running == nil {
		t.le.Debug("plugin is awaiting initial capability registration")
		t.c.setPluginStatus(
			t.pluginID,
			t.instanceKey,
			bldr_plugin.PluginState_PluginState_REQUESTED,
		)
		return
	}
	t.le.Debug("plugin rpc client and initial capabilities are ready")
	t.c.setPluginStatusClearingError(
		t.pluginID,
		t.instanceKey,
		bldr_plugin.PluginState_PluginState_RUNNING,
	)
}
