package plugin_host_scheduler

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
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
	installation     *installedManifests
	fallbacks        []*executePluginArgs
}

// executePluginArgsEqual compares two executePluginArgs for equality.
func executePluginArgsEqual(a, b *executePluginArgs) bool {
	// Compare nil args and the installation and fallback lists.
	if a == nil || b == nil {
		return a == b
	}
	if a.installation != b.installation || len(a.fallbacks) != len(b.fallbacks) {
		return false
	}
	for i, fallback := range a.fallbacks {
		if !executePluginArgsEqual(fallback, b.fallbacks[i]) {
			return false
		}
	}

	// Compare the manifest snapshots by executable manifest reference.
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

	// Compare the plugin host references.
	pluginHostEqual := (a.pluginHost == nil) == (b.pluginHost == nil)
	if pluginHostEqual && a.pluginHost != nil {
		pluginHostEqual = a.pluginHost == b.pluginHost
	}

	return pluginHostEqual
}

// execPlugin runs one immutable worker until its owned execution ends.
func (t *pluginInstance) execPlugin(ctx context.Context, args *executePluginArgs) (rerr error) {
	// Reject execution without a manifest snapshot and plugin host.
	if args == nil ||
		args.manifestSnapshot == nil ||
		args.manifestSnapshot.GetManifestRef() == nil ||
		args.pluginHost == nil {
		return nil
	}

	// Trace the execution and publish its result when it ends.
	ctx, task := trace.NewTask(ctx, "bldr/plugin-host-scheduler/execute-plugin")
	defer task.End()
	defer func() { t.finishExecution(rerr) }()
	t.ensureAccessProviders()
	defer func() {
		if rerr != nil {
			trace.Log(ctx, "manifest-copy-phase", "error")
			if !t.physical {
				t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "execute plugin", rerr)
			}
			return
		}
		if !t.physical {
			t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
		}
	}()

	// Prepare the manifest copy accounting and demand observation state.
	pluginManifest := args.manifestSnapshot
	pluginID, le := t.pluginID, t.le
	accounting := t.manifestCopyAccountingForExecution(ctx, pluginManifest)
	accessCtx := ctx
	var demandObservation *manifestDemandObservation
	var finishDemand func(string)

	// Log the plugin identity and startup fetch kind on the trace.
	trace.Log(ctx, "plugin-id", pluginID)
	trace.Log(ctx, "instance-key", t.instanceKey)
	trace.Log(ctx, "manifest-ref", pluginManifest.GetManifestRef().MarshalString())
	trace.Log(ctx, "startup-fetch-kind", "demand-plugin-execute")

	// Build a proxy volume over the host volume for the plugin.
	hostVol, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	proxyHostVol := volume_rpc_server.NewProxyVolume(ctx, hostVol.vol, false)

	// Wait for the World state handle.
	ws, err := t.c.worldStateCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Instrument the access context with block read accounting.
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

	// Access the manifest and run the plugin inside its dist.
	le.Infof("starting plugin with manifest: %s", pluginManifest.GetManifestRef().MarshalString())
	accessErr := manifest_world.AccessManifest(accessCtx, le, ws.AccessWorldState, pluginManifest.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS,
		assetsFS *unixfs.FSHandle,
	) error {
		// Retain executable identity even when startup bypasses the catalog store.
		// Exact replay needs the reference, without requiring an eager DAG copy.
		if t.manifestRoot == "" && (t.c.conf.GetDisableStoreManifest() || args.installation != nil) {
			ref := bldr_manifest.NewManifestRef(manifest.GetMeta(), pluginManifest.GetManifestRef())
			if err := manifest_world.ExStoreManifestOp(ctx, ws, t.c.peerID,
				bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef()), []string{t.c.objKey}, ref); err != nil {
				return err
			}
		}

		// Report whether the first demanded block was read during access.
		if demandObservation != nil {
			snapshot := demandObservation.snapshot()
			if snapshot.BlockReadCount != 0 {
				finishDemand("first-demand-block")
			} else {
				finishDemand("access-manifest-ready")
			}
		}

		// Serve the dist and assets filesystems while the plugin runs.
		t.distAccess.SetCurrent(unixfs_access.NewAccessUnixFSFunc(distFS))
		defer t.distAccess.SetBlocked()
		t.assetsAccess.SetCurrent(unixfs_access.NewAccessUnixFSFunc(assetsFS))
		defer t.assetsAccess.SetBlocked()
		manifestRoot := pluginManifest.GetManifestRef().GetRootRef().GetHash().MarshalString()

		// Publish the immutable dist and assets file controllers.
		// Current executions also publish immutable files. A viewer or module URL
		// must never silently resolve to a later revision with the same plugin ID.
		if t.manifestRoot == "" {
			artifactID := bldr_plugin.PluginArtifactID(pluginID, manifestRoot)
			for _, files := range []struct {
				id string
				fs *unixfs.FSHandle
			}{
				{bldr_plugin.PluginDistFsId(artifactID), distFS},
				{bldr_plugin.PluginAssetsFsId(artifactID), assetsFS},
			} {
				ctrl := unixfs_access.NewController(t.le, t.c.bus,
					controller.NewInfo(ControllerID+"/"+files.id, Version, "immutable plugin files"),
					[]string{files.id}, unixfs_access.NewAccessUnixFSFunc(files.fs))
				defer ctrl.Close()
				release, err := t.c.bus.AddController(ctx, ctrl, nil)
				if err != nil {
					return err
				}
				defer release()
			}
		}

		// Announce the root only once its files are served. The browser retries
		// fetches lost with a runtime after the next runtime announces the root.
		if !t.physical {
			t.emitPluginManifestRoot(manifestRoot)
		}

		// Hold each dependency while this plugin runs. Dependencies start
		// alongside it: an import of a dependency's web package waits in
		// LookupWebPkg until the provider forwards it.
		for _, dep := range manifest.GetDeps() {
			_, depRef, err := t.c.bus.AddDirective(bldr_plugin.NewLoadPlugin(dep), nil)
			if err != nil {
				return err
			}
			defer depRef.Release()
		}

		// Resolve the plugin host root for the platform.
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

		// Begin the initial capability registration window.
		t.beginInitialCapabilityRegistration()

		// Build the mux for handling incoming RPCs from the plugin.
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
			t.manifestRoot != "",
			t.prepared,
			t.bindingKey,
		)
		defer relHostMux()

		// Execute the plugin on the resolved host until it stops.
		execErr := args.pluginHost.ExecutePlugin(
			ctx,
			pluginID,
			t.bindingKey,
			t.instanceKey,
			manifestRoot,
			manifest.GetEntrypoint(),
			distFS,
			assetsFS,
			hostMux,
			func(client srpc.Client) error { t.updateRpcClient(client); return nil },
		)

		// Map a canceled context to context.Canceled; otherwise surface the error.
		if execErr != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}

			le.WithError(execErr).Error("plugin execution errored")
			return execErr
		}

		return nil
	})

	// Finalize the demand observation phase after access completes.
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
	// Reset the load state to the pending registration phase.
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
	// Publish the new rpc client, marking failed registration on disconnect.
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		// A disconnect during registration precedes process Wait. Preserve pending
		// until ExecutePlugin returns the diagnostic that decides retry policy.
		registrationState := current.GetInitialCapabilityRegistrationState()
		if client == nil && registrationState == bldr_plugin.InitialCapabilityRegistrationComplete {
			registrationState = bldr_plugin.InitialCapabilityRegistrationFailed
		}
		next := bldr_plugin.NewPluginLoadState(client, registrationState)
		if current.GetStartupBudgetExhausted() {
			next = next.WithStartupBudgetExhausted()
		}
		return next
	})
}

// finishExecution publishes the execution result after the process has stopped.
func (t *pluginInstance) finishExecution(err error) {
	// Record the execution result in the load state.
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		return current.WithStartupError(err)
	})
}

// finishInitialCapabilityRegistration publishes the plugin's startup RPC result.
func (t *pluginInstance) finishInitialCapabilityRegistration(complete bool) {
	// Publish the registration result in the load state.
	t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		// Select the registration state from the completion flag.
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

	// Stop the startup wait budget once registration completes.
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
	// Swap the load state and publish the projection in one critical section.
	return t.pluginLoadStateCtr.SwapValue(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		next := cb(current)
		t.publishPluginLoadState(next)
		return next
	})
}

// publishPluginLoadState updates the running projection inside the load-state lock.
func (t *pluginInstance) publishPluginLoadState(state bldr_plugin.PluginLoadState) {
	// Publish the running plugin and notify the load state listener.
	running := state.GetRunningPlugin()
	t.runningPluginCtr.SetValue(running)
	if t.onLoadState != nil {
		t.onLoadState(state)
	}

	// Report the plugin status for non-physical instances.
	if t.physical {
		return
	}
	if running == nil {
		t.le.Debug("plugin is awaiting initial capability registration")
		t.c.setPluginStatus(
			t.pluginID,
			t.instanceKey,
			bldr_plugin.PluginState_PluginState_REQUESTED,
		)
		return
	}

	// Report the running status once the rpc client is ready.
	t.le.Debug("plugin rpc client and initial capabilities are ready")
	t.c.setPluginStatusClearingError(
		t.pluginID,
		t.instanceKey,
		bldr_plugin.PluginState_PluginState_RUNNING,
	)
}
