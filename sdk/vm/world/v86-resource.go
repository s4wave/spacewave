package s4wave_vm_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// defaultVmPluginID is the plugin ID that hosts the default v86 backend. The vm backend is
// folded into spacewave-app; each VmV86 object gets its own SharedWorker
// instance of spacewave-app keyed by the object key.
const defaultVmPluginID = "spacewave-app"

// v86Resource implements PersistentExecutionService for a VmV86 object.
type v86Resource struct {
	objectKey string
	ws        world.WorldState
	b         bus.Bus
}

// newV86Resource constructs a new v86Resource.
func newV86Resource(objectKey string, ws world.WorldState, b bus.Bus) *v86Resource {
	return &v86Resource{objectKey: objectKey, ws: ws, b: b}
}

// Execute implements SRPCPersistentExecutionServiceServer.
//
// Reads the requested state from the VmV86 block and reconciles the plugin
// lifecycle to match:
//   - STARTING / RUNNING: verify the rootfs mount resolves, load the plugin
//     SharedWorker, emit RUNNING. Mount or plugin load failure emits ERROR and
//     leaves the handler idle until the stored state changes again.
//   - STOPPED / STOPPING / ERROR: release any held plugin ref, emit the matching
//     status.
//
// Reacts to SetV86StateOp by waiting on the object's revision via WaitRev;
// every transition flips the stored state and wakes the handler.
func (r *v86Resource) Execute(req *s4wave_process.ExecuteRequest, stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream) error {
	ctx := stream.Context()

	var rpRef directive.Reference
	defer func() {
		if rpRef != nil {
			rpRef.Release()
		}
	}()

	// sentinel means "nothing emitted yet"; any real state will differ.
	lastEmitted := s4wave_process.ExecutionState(-1)
	emit := func(s s4wave_process.ExecutionState) error {
		if s == lastEmitted {
			return nil
		}
		if err := stream.Send(&s4wave_process.ExecuteStatus{State: s}); err != nil {
			return err
		}
		lastEmitted = s
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		objState, found, err := r.ws.GetObject(ctx, r.objectKey)
		if err != nil {
			return err
		}
		if !found {
			if rpRef != nil {
				rpRef.Release()
				rpRef = nil
			}
			if err := emit(s4wave_process.ExecutionState_ExecutionState_STOPPED); err != nil {
				return err
			}
			return nil
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		storedState := s4wave_vm.VmState_VmState_STOPPED
		runtimePluginID := defaultVmPluginID
		_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
			vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
				return &s4wave_vm.VmV86{}
			})
			if unmarshalErr != nil {
				return unmarshalErr
			}
			if vm != nil {
				storedState = vm.GetState()
				if pluginID := vm.GetConfig().GetRuntimePluginId(); pluginID != "" {
					runtimePluginID = pluginID
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		desired := mapVmState(storedState)
		switch desired {
		case s4wave_process.ExecutionState_ExecutionState_STARTING,
			s4wave_process.ExecutionState_ExecutionState_RUNNING:
			if rpRef == nil {
				if err := emit(s4wave_process.ExecutionState_ExecutionState_STARTING); err != nil {
					return err
				}
				if mountErr := r.verifyRootfsMount(ctx); mountErr != nil {
					if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR); err != nil {
						return err
					}
				} else if homeErr := ensureHomeMount(ctx, r.ws, r.objectKey); homeErr != nil {
					if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR); err != nil {
						return err
					}
				} else {
					// returnIfIdle=true so missing plugin hosts surface as a
					// nil value rather than blocking the handler forever.
					plugin, _, newRef, loadErr := bldr_plugin.ExLoadPluginInstanced(ctx, r.b, true, runtimePluginID, r.objectKey, nil)
					if loadErr != nil || plugin == nil {
						if newRef != nil {
							newRef.Release()
						}
						if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR); err != nil {
							return err
						}
					} else {
						rpRef = newRef
						if err := emit(s4wave_process.ExecutionState_ExecutionState_RUNNING); err != nil {
							rpRef.Release()
							rpRef = nil
							return err
						}
					}
				}
			} else {
				if err := emit(s4wave_process.ExecutionState_ExecutionState_RUNNING); err != nil {
					return err
				}
			}
		default:
			if rpRef != nil {
				rpRef.Release()
				rpRef = nil
			}
			if err := emit(desired); err != nil {
				return err
			}
		}

		if _, err := objState.WaitRev(ctx, rev+1, false); err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			if errors.Is(err, world.ErrObjectNotFound) {
				if rpRef != nil {
					rpRef.Release()
					rpRef = nil
				}
				return nil
			}
			return err
		}
	}
}

// verifyRootfsMount confirms the rootfs asset (empty mount name) resolves
// through the V86Image or override edge before the plugin is loaded. Any
// failure here is treated as an ERROR state for the handler.
func (r *v86Resource) verifyRootfsMount(ctx context.Context) error {
	fsh, err := resolveV86Mount(ctx, r.ws, r.objectKey, "")
	if err != nil {
		return err
	}
	fsh.Release()
	return nil
}

// mapVmState maps VmState to ExecutionState.
func mapVmState(vs s4wave_vm.VmState) s4wave_process.ExecutionState {
	switch vs {
	case s4wave_vm.VmState_VmState_STARTING:
		return s4wave_process.ExecutionState_ExecutionState_STARTING
	case s4wave_vm.VmState_VmState_RUNNING:
		return s4wave_process.ExecutionState_ExecutionState_RUNNING
	case s4wave_vm.VmState_VmState_STOPPING:
		return s4wave_process.ExecutionState_ExecutionState_STOPPING
	case s4wave_vm.VmState_VmState_ERROR:
		return s4wave_process.ExecutionState_ExecutionState_ERROR
	default:
		return s4wave_process.ExecutionState_ExecutionState_STOPPED
	}
}

// _ is a type assertion
var _ s4wave_process.SRPCPersistentExecutionServiceServer = (*v86Resource)(nil)
