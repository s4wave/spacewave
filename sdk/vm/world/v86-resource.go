package s4wave_vm_world

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/block"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/s4wave/spacewave/db/world"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	"github.com/sirupsen/logrus"
)

// defaultVmPluginID is the plugin ID that hosts the default V86 backend. Each
// VmV86 object gets its own plugin instance keyed by the object key.
const defaultVmPluginID = "spacewave-v86"

const v86RuntimeV86fsServicePrefix = "vm/v86-runtime/v86fs/"

// v86Resource implements PersistentExecutionService for a VmV86 object.
type v86Resource struct {
	objectKey   string
	ws          world.WorldState
	b           bus.Bus
	v86fsServer unixfs_v86fs.SRPCV86FsServiceServer
}

// newV86Resource constructs a new v86Resource.
func newV86Resource(objectKey string, ws world.WorldState, b bus.Bus, v86fsServer unixfs_v86fs.SRPCV86FsServiceServer) *v86Resource {
	return &v86Resource{objectKey: objectKey, ws: ws, b: b, v86fsServer: v86fsServer}
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
	var v86fsRouteRelease func()
	defer func() {
		if rpRef != nil {
			rpRef.Release()
		}
		if v86fsRouteRelease != nil {
			v86fsRouteRelease()
		}
	}()

	// sentinel means "nothing emitted yet"; any real state will differ.
	lastEmitted := s4wave_process.ExecutionState(-1)
	emit := func(s s4wave_process.ExecutionState, errMsg string) error {
		if s == lastEmitted {
			return nil
		}
		if err := stream.Send(&s4wave_process.ExecuteStatus{State: s, Error: errMsg}); err != nil {
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
			if v86fsRouteRelease != nil {
				v86fsRouteRelease()
				v86fsRouteRelease = nil
			}
			if err := emit(s4wave_process.ExecutionState_ExecutionState_STOPPED, ""); err != nil {
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
				if err := emit(s4wave_process.ExecutionState_ExecutionState_STARTING, ""); err != nil {
					return err
				}
				if mountErr := r.verifyRootfsMount(ctx); mountErr != nil {
					if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, mountErr.Error()); err != nil {
						return err
					}
				} else if homeErr := ensureHomeMount(ctx, r.ws, r.objectKey); homeErr != nil {
					if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, homeErr.Error()); err != nil {
						return err
					}
				} else {
					_, rev, err = objState.GetRootRef(ctx)
					if err != nil {
						return err
					}
					newV86fsRouteRelease, routeErr := r.exposeV86fsToRuntimePlugin(ctx, runtimePluginID)
					if routeErr != nil {
						if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, routeErr.Error()); err != nil {
							return err
						}
						break
					}
					// returnIfIdle=true so missing plugin hosts surface as a
					// nil value rather than blocking the handler forever.
					plugin, _, newRef, loadErr := bldr_plugin.ExLoadPluginInstanced(ctx, r.b, true, runtimePluginID, r.objectKey, nil)
					if loadErr != nil || plugin == nil {
						if newRef != nil {
							newRef.Release()
						}
						newV86fsRouteRelease()
						errMsg := "v86 runtime plugin unavailable"
						if loadErr != nil {
							errMsg = loadErr.Error()
						}
						if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, errMsg); err != nil {
							return err
						}
					} else {
						rpRef = newRef
						v86fsRouteRelease = newV86fsRouteRelease
						loadedState := desired
						if storedState == s4wave_vm.VmState_VmState_STARTING {
							loadedState = s4wave_process.ExecutionState_ExecutionState_STARTING
						}
						if err := emit(loadedState, ""); err != nil {
							rpRef.Release()
							rpRef = nil
							return err
						}
					}
				}
			} else {
				loadedState := desired
				if storedState == s4wave_vm.VmState_VmState_STARTING {
					loadedState = s4wave_process.ExecutionState_ExecutionState_STARTING
				}
				if err := emit(loadedState, ""); err != nil {
					return err
				}
			}
		default:
			if rpRef != nil {
				rpRef.Release()
				rpRef = nil
			}
			if v86fsRouteRelease != nil {
				v86fsRouteRelease()
				v86fsRouteRelease = nil
			}
			if err := emit(desired, ""); err != nil {
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
				if v86fsRouteRelease != nil {
					v86fsRouteRelease()
					v86fsRouteRelease = nil
				}
				return nil
			}
			return err
		}
	}
}

func (r *v86Resource) exposeV86fsToRuntimePlugin(ctx context.Context, pluginID string) (func(), error) {
	servicePrefix := v86RuntimeV86fsServicePrefix + r.objectKey + "/"
	mux := srpc.NewMux(nil)
	if err := unixfs_v86fs.SRPCRegisterV86FsService(mux, r.v86fsServer); err != nil {
		return nil, err
	}
	pluginServerID := bldr_plugin.PluginServerID(pluginID, "")
	workerServerID := "web-worker/" + bldr_plugin.PluginServerID(pluginID, r.objectKey)
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			"sdk/vm/world/v86/"+r.objectKey+"/v86fs-runtime-route",
			controller.MustParseVersion("0.0.1"),
			"v86fs route for VmV86 runtime plugin",
		),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return v86fsRuntimeRouteInvoker{inv: mux}, nil, nil
		},
		[]string{servicePrefix},
		true,
		nil,
		nil,
		regexp.MustCompile("^(?:"+regexp.QuoteMeta(pluginServerID)+"|"+regexp.QuoteMeta(workerServerID)+")$"),
	)
	release, err := r.b.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		return nil, err
	}
	return release, nil
}

type v86fsRuntimeRouteInvoker struct {
	inv srpc.Invoker
}

func (i v86fsRuntimeRouteInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ok, err := i.inv.InvokeMethod(serviceID, methodID, strm)
	logrus.WithError(err).
		WithField("service-id", serviceID).
		WithField("method-id", methodID).
		WithField("ok", ok).
		Debug("v86fs runtime route invoke")
	return ok, err
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
