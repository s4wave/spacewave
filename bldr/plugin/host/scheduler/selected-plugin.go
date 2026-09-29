package plugin_host_scheduler

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/bucket"
)

// executionReference identifies a candidate and its registration startup mode.
type executionReference struct {
	args     *executePluginArgs
	prepared bool
}

// newExecution constructs one immutable worker without manifest-selection loops.
func (t *pluginInstance) newExecution(key executionReference) (keyed.Routine, *pluginInstance) {
	// Build a physical worker keyed by the candidate's manifest root.
	root := manifestRootKey(key.args.manifestSnapshot.GetManifestRef())
	worker := newPluginState(t.c, t.le, t.pluginID, t.instanceKey+"/generation/"+root, t.manifestRoot)
	worker.bindingKey = t.bindingKey
	worker.physical = true
	worker.prepared = key.prepared
	worker.onLoadState = func(state plugin.PluginLoadState) {
		t.pluginUpdateMtx.Lock()
		defer t.pluginUpdateMtx.Unlock()
		if t.activeExecution == worker {
			t.updatePluginLoadState(func(plugin.PluginLoadState) plugin.PluginLoadState { return state })
		}
	}
	return func(ctx context.Context) error { return worker.execPlugin(ctx, key.args) }, worker
}

// execSelectedPlugin recovers a cold installation with its retained artifacts.
// An admitted worker survives failed replacements without restarting or falling
// back. Manifests that fail startup with a protocol mismatch are rejected for
// this binding, and World selection repeats without them.
func (t *pluginInstance) execSelectedPlugin(ctx context.Context, args *executePluginArgs) error {
	// Try the selection, then its retained fallbacks while no worker runs. A
	// protocol mismatch rejects that manifest on this host and moves on.
	candidates := []*executePluginArgs{args}
	if args != nil {
		candidates = append(candidates, args.fallbacks...)
	}
	var err error
	var protocolErr *plugin_host.StartupProtocolError
	for i, candidate := range candidates {
		if i != 0 && (err == nil || ctx.Err() != nil || t.runningPluginCtr.GetValue() != nil) {
			break
		}
		err = t.execSelectedCandidate(ctx, candidate, i != 0)
		if errors.As(err, &protocolErr) {
			t.incompatibleManifests.Store(manifestRootKey(candidate.manifestSnapshot.GetManifestRef()), struct{}{})
		}
	}
	if !errors.As(err, &protocolErr) {
		return err
	}

	// A proven protocol mismatch is terminal for these candidates. Returning nil
	// stops backoff retries; World selection repeats without the rejected roots.
	t.stopStartupWaitBudget()
	if t.runningPluginCtr.GetValue() == nil {
		t.finishExecution(err)
	}
	if args != nil && args.installation == nil {
		t.manifestSelectionFingerprint.Store(nil)
		t.watchWorldManifestRoutine.RestartRoutine()
	}
	return nil
}

// manifestRootKey identifies a manifest's executable content across buckets.
func manifestRootKey(ref *bucket.ObjectRef) string {
	return ref.GetRootRef().GetHash().MarshalString()
}

// incompatibleManifest reports whether the manifest failed startup on this host.
func (t *pluginInstance) incompatibleManifest(ref *bucket.ObjectRef) bool {
	_, ok := t.incompatibleManifests.Load(manifestRootKey(ref))
	return ok
}

// execSelectedCandidate prepares a candidate while retaining the admitted worker.
// Legacy plugins retain their in-place replacement contract; typed plugins
// publish their complete registration scope only after startup succeeds.
func (t *pluginInstance) execSelectedCandidate(ctx context.Context, args *executePluginArgs, recovery bool) (rerr error) {
	// Reject a candidate the binding no longer accepts.
	if !t.acceptsManifest(args) {
		return context.Canceled
	}

	// An explicit removal releases the binding; candidate cancellation does not.
	if args == nil || args.manifestSnapshot == nil {
		t.clearExecution(nil)
		return nil
	}
	defer func() {
		if rerr != nil && ctx.Err() == nil {
			t.c.recordPluginStatusError(t.pluginID, t.instanceKey, "prepare plugin", rerr)
		}
	}()
	if args.pluginHost == nil {
		return errors.New("installed plugin has no compatible host")
	}

	// Probe the current worker's declared admission contract. Go and JavaScript
	// RPC servers report an absent method differently. Transport failures preserve
	// the worker; an absent activation method selects in-place replacement.
	prepared := false
	if current := t.runningPluginCtr.GetValue(); current != nil && t.manifestRoot == "" {
		_, err := plugin.NewSRPCActivationClient(current.GetRpcClient()).Check(ctx, &plugin.CheckActivationRequest{})
		if err != nil && err.Error() != srpc.ErrUnimplemented.Error() &&
			err.Error() != "not found: bldr.plugin.Activation/Check" {
			return err
		}
		prepared = err == nil
	}
	if !prepared {
		t.clearExecution(nil)
	}

	// The logical binding owns workers beyond this selection attempt. Until
	// admission, canceling or failing this attempt releases only the candidate.

	// Register the candidate as an execution and admit it when unprepared.
	ref, worker, _ := t.executions.AddKeyRef(executionReference{args: args, prepared: prepared})
	admitted := false
	defer func() {
		if !admitted {
			ref.Release()
		}
	}()
	if !prepared {
		admitted = t.admitExecution(worker, ref.Release, args)
		if !admitted {
			return context.Canceled
		}
	}

	// Wait for startup to complete or fail before activation.
	state, err := worker.pluginLoadStateCtr.WaitValueWithValidator(ctx, func(state plugin.PluginLoadState) (bool, error) {
		if state.GetInitialCapabilityRegistrationState() == plugin.InitialCapabilityRegistrationFailed {
			if err := state.GetStartupError(); err != nil {
				return false, err
			}
			return false, errors.New("plugin exited before completing startup")
		}
		return state.GetRunningPlugin() != nil, nil
	}, nil)
	if err != nil {
		if admitted && ctx.Err() == nil {
			t.clearExecution(worker)
		}
		return err
	}

	// Activate only a ready candidate. Its attached handlers and immutable viewer
	// URLs are already usable before the family RPC alias changes.
	if prepared {
		if !t.acceptsManifest(args) {
			return context.Canceled
		}
		activation := plugin.NewSRPCActivationClient(state.GetRpcClient())
		if _, err := activation.Activate(ctx, &plugin.ActivatePluginRequest{}); err != nil {
			return err
		}
		admitted = t.admitExecution(worker, ref.Release, args)
		if !admitted {
			return context.Canceled
		}
	}

	// Publish the admitted manifest root and clear startup bookkeeping.
	t.emitPluginManifestRoot(args.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
	if !recovery {
		t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
	}
	t.stopStartupWaitBudget()

	// The worker's callback keeps public state current even while another
	// candidate is preparing. A crashed admitted worker is released before retry.

	// Wait for the admitted worker to report a failed registration.
	_, err = worker.pluginLoadStateCtr.WaitValueWithValidator(ctx, func(state plugin.PluginLoadState) (bool, error) {
		return state.GetInitialCapabilityRegistrationState() == plugin.InitialCapabilityRegistrationFailed, nil
	}, nil)
	if err != nil {
		return err
	}
	t.clearExecution(worker)
	return errors.New("admitted plugin exited")
}

// admitExecution switches the family alias and filesystem access to a worker.
// Its load-state lock precedes pluginUpdateMtx, matching the worker callback.
func (t *pluginInstance) admitExecution(worker *pluginInstance, release func(), args *executePluginArgs) bool {
	// Swap the admitted worker under the load-state lock.
	var previous func()
	var admitted bool
	worker.pluginLoadStateCtr.SwapValue(func(state plugin.PluginLoadState) plugin.PluginLoadState {
		// Admit the worker only while the binding is open and accepts the manifest.
		t.pluginUpdateMtx.Lock()
		defer t.pluginUpdateMtx.Unlock()
		if t.executionsClosed || !t.acceptsManifest(args) {
			return state
		}
		admitted = true
		previous = t.releaseExecution
		t.activeExecution = worker
		t.releaseExecution = release

		// Switch the family alias and file access to the new worker.
		t.ensureAccessProviders()
		t.distAccess.SetCurrent(worker.distAccess.AccessUnixFS)
		t.assetsAccess.SetCurrent(worker.assetsAccess.AccessUnixFS)
		t.updatePluginLoadState(func(plugin.PluginLoadState) plugin.PluginLoadState { return state })
		return state
	})

	// Release the previously admitted worker after the swap commits.
	if previous != nil {
		previous()
	}
	return admitted
}

// closeExecutions prevents late admission after the logical binding closes.
func (t *pluginInstance) closeExecutions() {
	t.pluginUpdateMtx.Lock()
	t.executionsClosed = true
	t.pluginUpdateMtx.Unlock()
	t.clearExecution(nil)
}

// clearExecution releases the admitted worker and clears the logical binding.
func (t *pluginInstance) clearExecution(expected *pluginInstance) {
	// Return unchanged when no worker or a different worker is admitted.
	t.pluginUpdateMtx.Lock()
	if t.activeExecution == nil || expected != nil && t.activeExecution != expected {
		t.pluginUpdateMtx.Unlock()
		return
	}

	// Clear the admitted worker and block file access until the next admission.
	release := t.releaseExecution
	t.releaseExecution = nil
	t.activeExecution = nil
	t.ensureAccessProviders()
	t.distAccess.SetBlocked()
	t.assetsAccess.SetBlocked()
	t.beginInitialCapabilityRegistration()
	t.pluginUpdateMtx.Unlock()

	// Release the replaced worker outside the update lock.
	if release != nil {
		release()
	}
}
