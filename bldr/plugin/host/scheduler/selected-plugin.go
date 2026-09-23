package plugin_host_scheduler

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// executionReference identifies a candidate and its registration startup mode.
type executionReference struct {
	args     *executePluginArgs
	prepared bool
}

// newExecution constructs one immutable worker without manifest-selection loops.
func (t *pluginInstance) newExecution(key executionReference) (keyed.Routine, *pluginInstance) {
	root := key.args.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString()
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
// An admitted worker survives failed replacements without restarting or falling back.
func (t *pluginInstance) execSelectedPlugin(ctx context.Context, args *executePluginArgs) error {
	err := t.execSelectedCandidate(ctx, args, false)
	if args == nil {
		return err
	}
	for _, fallback := range args.fallbacks {
		if err == nil || ctx.Err() != nil || t.runningPluginCtr.GetValue() != nil {
			return err
		}
		err = t.execSelectedCandidate(ctx, fallback, true)
	}
	return err
}

// execSelectedCandidate prepares a candidate while retaining the admitted worker.
// Legacy plugins retain their in-place replacement contract; typed plugins
// publish their complete registration scope only after startup succeeds.
func (t *pluginInstance) execSelectedCandidate(ctx context.Context, args *executePluginArgs, recovery bool) (rerr error) {
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
	state, err := worker.pluginLoadStateCtr.WaitValueWithValidator(ctx, func(state plugin.PluginLoadState) (bool, error) {
		if state.GetInitialCapabilityRegistrationState() == plugin.InitialCapabilityRegistrationFailed {
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
	t.emitPluginManifestRoot(args.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
	if !recovery {
		t.c.clearPluginStatusError(t.pluginID, t.instanceKey)
	}
	t.stopStartupWaitBudget()

	// The worker's callback keeps public state current even while another
	// candidate is preparing. A crashed admitted worker is released before retry.
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
	var previous func()
	var admitted bool
	worker.pluginLoadStateCtr.SwapValue(func(state plugin.PluginLoadState) plugin.PluginLoadState {
		t.pluginUpdateMtx.Lock()
		defer t.pluginUpdateMtx.Unlock()
		if t.executionsClosed || !t.acceptsManifest(args) {
			return state
		}
		admitted = true
		previous = t.releaseExecution
		t.activeExecution = worker
		t.releaseExecution = release
		t.ensureAccessProviders()
		t.distAccess.SetCurrent(worker.distAccess.AccessUnixFS)
		t.assetsAccess.SetCurrent(worker.assetsAccess.AccessUnixFS)
		t.updatePluginLoadState(func(plugin.PluginLoadState) plugin.PluginLoadState { return state })
		return state
	})
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
	t.pluginUpdateMtx.Lock()
	if t.activeExecution == nil || expected != nil && t.activeExecution != expected {
		t.pluginUpdateMtx.Unlock()
		return
	}
	release := t.releaseExecution
	t.releaseExecution = nil
	t.activeExecution = nil
	t.ensureAccessProviders()
	t.distAccess.SetBlocked()
	t.assetsAccess.SetBlocked()
	t.beginInitialCapabilityRegistration()
	t.pluginUpdateMtx.Unlock()
	if release != nil {
		release()
	}
}
