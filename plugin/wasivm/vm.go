package wasivm_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// vmResource runs one WasiVM object. The Space calls Execute on one resource
// per VM. The resource boots the kernel and publishes its console in consoles.
type vmResource struct {
	le *logrus.Entry
	// b mounts the CDN Space of a published image.
	b         bus.Bus
	objectKey string
	ws        world.WorldState
	consoles  *consoleTable
}

// run is one boot of a VM.
type run struct {
	// generation is the run generation the boot belongs to.
	generation uint64
	// cancel stops the kernel.
	cancel context.CancelFunc
	// unpublish removes the console from the console table.
	unpublish func()
}

// stop removes the console and stops the kernel.
func (r *run) stop() {
	r.unpublish()
	r.cancel()
}

// Execute reconciles the desired state of the VM with a kernel booted in this
// process.
//
// Starting a run boots the kernel on the linked root filesystem and publishes
// its console, then commits RUNNING. A run that cannot start commits ERROR.
// When init exits or the system halts, the run commits STOPPED as both the
// desired and observed state. When the run fails, it commits ERROR. When the
// system restarts, the run begins the next generation, which this loop then
// starts. Every commit is fenced by the run generation, so a late report from
// an earlier run changes nothing.
func (r *vmResource) Execute(req *s4wave_process.ExecuteRequest, stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream) error {
	// Stop the current run when its generation ends or execution ends.
	var cur *run
	stopRun := func() {
		if cur != nil {
			cur.stop()
			cur = nil
		}
	}
	defer stopRun()

	// Send each change of execution state, and every error.
	lastEmitted := s4wave_process.ExecutionState(-1)
	emit := func(s s4wave_process.ExecutionState, errMsg string) error {
		if s == lastEmitted && errMsg == "" {
			return nil
		}
		if err := stream.Send(&s4wave_process.ExecuteStatus{State: s, Error: errMsg}); err != nil {
			return err
		}
		lastEmitted = s
		return nil
	}

	// Open the VM object. A missing object is stopped.
	ctx := stream.Context()
	objState, found, err := r.ws.GetObject(ctx, r.objectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return err
	}
	if !found {
		return emit(s4wave_process.ExecutionState_ExecutionState_STOPPED, "")
	}

	// Reconcile the run against each revision of the VM object.
	for {
		// Read the VM and drop a run of an earlier generation.
		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}
		vm, err := readVm(ctx, objState)
		if err != nil {
			return err
		}
		generation := vm.GetRunGeneration()
		if cur != nil && cur.generation != generation {
			stopRun()
		}

		// Move the run toward the desired state.
		switch vm.GetState() {
		case s4wave_vm.VmState_VmState_RUNNING, s4wave_vm.VmState_VmState_STARTING:
			if vm.GetObservedState() == s4wave_vm.VmState_VmState_ERROR {
				stopRun()
				if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, vm.GetErrorMessage()); err != nil {
					return err
				}
				break
			}
			if cur == nil {
				cur, err = r.startRun(ctx, generation, vm.GetConfig())
				if err != nil {
					if _, _, err := r.setObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, err.Error()); err != nil {
						return err
					}
					continue
				}
			}
			changed, accepted, err := r.setObservedState(ctx, generation, s4wave_vm.VmState_VmState_RUNNING, "")
			if err != nil {
				return err
			}
			if changed || !accepted {
				continue
			}
			if err := emit(s4wave_process.ExecutionState_ExecutionState_RUNNING, ""); err != nil {
				return err
			}
		default:
			stopRun()
			changed, accepted, err := r.setObservedState(ctx, generation, s4wave_vm.VmState_VmState_STOPPED, "")
			if err != nil {
				return err
			}
			if changed || !accepted {
				continue
			}
			if err := emit(s4wave_process.ExecutionState_ExecutionState_STOPPED, ""); err != nil {
				return err
			}
		}

		// Wait for the next revision of the VM object.
		if _, err := objState.WaitRev(ctx, rev+1, false); err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			if errors.Is(err, world.ErrObjectNotFound) {
				return nil
			}
			return err
		}
	}
}

// readVm reads the WasiVm block of a VM object.
func readVm(ctx context.Context, objState world.ObjectState) (*s4wave_vm.WasiVm, error) {
	var vm *s4wave_vm.WasiVm
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		vm, err = block.UnmarshalBlock[s4wave_vm.WasiVm](ctx, bcs)
		return err
	})
	if err == nil && vm == nil {
		err = errors.New("the object holds no WasiVM")
	}
	return vm, err
}

// startRun boots the kernel for a run generation and publishes its console.
func (r *vmResource) startRun(ctx context.Context, generation uint64, config *s4wave_vm.WasiVmConfig) (*run, error) {
	// Find the root filesystem of the VM for the length of the run.
	runCtx, cancel := context.WithCancel(ctx)
	rootfs, err := resolveRootfs(runCtx, r.le, r.b, r.ws, r.objectKey)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "find the root filesystem")
	}

	// Boot the kernel. A run that ends on its own commits how it ended.
	console, err := startKernel(runCtx, r.le, rootfs, config, func(reboot bool, runErr error) {
		if runCtx.Err() == nil {
			r.finishRun(runCtx, generation, reboot, runErr)
		}
	})
	if err != nil {
		cancel()
		return nil, err
	}
	return &run{generation: generation, cancel: cancel, unpublish: r.consoles.publish(r.objectKey, console)}, nil
}

// finishRun commits the end of a run: a new run generation when the system
// restarted, STOPPED as both the desired and observed state when init exited
// or the system halted, or ERROR when the run failed.
func (r *vmResource) finishRun(ctx context.Context, generation uint64, reboot bool, runErr error) {
	_, _, err := r.updateRun(ctx, generation, func(vm *s4wave_vm.WasiVm) bool {
		// A restart begins the next run, which Execute starts.
		if reboot {
			vm.RunGeneration++
			vm.ObservedState = s4wave_vm.VmState_VmState_STARTING
			vm.ErrorMessage = ""
			return true
		}

		// A failed run keeps the desired state for a retry.
		if runErr != nil {
			vm.ObservedState = s4wave_vm.VmState_VmState_ERROR
			vm.ErrorMessage = runErr.Error()
			return true
		}

		// An exited init stops the VM.
		vm.State = s4wave_vm.VmState_VmState_STOPPED
		vm.ObservedState = s4wave_vm.VmState_VmState_STOPPED
		vm.ErrorMessage = ""
		return true
	})
	if err != nil && ctx.Err() == nil {
		r.le.WithError(err).Warn("unable to record the end of a run")
	}
}

// setObservedState commits the observed state of a run generation.
func (r *vmResource) setObservedState(
	ctx context.Context,
	generation uint64,
	state s4wave_vm.VmState,
	errorMessage string,
) (changed bool, accepted bool, err error) {
	if state != s4wave_vm.VmState_VmState_ERROR {
		errorMessage = ""
	}
	return r.updateRun(ctx, generation, func(vm *s4wave_vm.WasiVm) bool {
		if vm.GetObservedState() == state && vm.GetErrorMessage() == errorMessage {
			return false
		}
		vm.ObservedState = state
		vm.ErrorMessage = errorMessage
		return true
	})
}

// updateRun applies update to the VM block when its run generation matches
// generation. update reports whether it changed the block. accepted reports
// whether the generation matched.
func (r *vmResource) updateRun(
	ctx context.Context,
	generation uint64,
	update func(vm *s4wave_vm.WasiVm) bool,
) (changed bool, accepted bool, err error) {
	// Open the VM object.
	objState, found, err := r.ws.GetObject(ctx, r.objectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, world.ErrObjectNotFound
	}

	// Update the VM block when the run generation matches.
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		// Read the block, leaving another generation's run untouched.
		vm, err := block.UnmarshalBlock[s4wave_vm.WasiVm](ctx, bcs)
		if err != nil || vm == nil || vm.GetRunGeneration() != generation {
			return err
		}

		// Apply the update, writing the block only when it changed.
		accepted = true
		if changed = update(vm); changed {
			bcs.SetBlock(vm, true)
		}
		return nil
	})
	return changed, accepted, err
}

// _ is a type assertion
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*vmResource)(nil)
