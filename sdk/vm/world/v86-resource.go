package s4wave_vm_world

import (
	"context"
	"regexp"
	"sync"

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

const (
	v86RuntimeV86fsServicePrefix  = "vm/v86-runtime/v86fs/"
	v86RuntimeStatusServicePrefix = "vm/v86-runtime/status/"
)

// v86Resource implements PersistentExecutionService for a VmV86 object.
type v86Resource struct {
	le          *logrus.Entry
	objectKey   string
	ws          world.WorldState
	b           bus.Bus
	v86fsServer *unixfs_v86fs.Server

	homeMountMtx     sync.Mutex
	homeMountRefs    uint
	homeMountCleanup func()
}

// newV86Resource constructs a new v86Resource.
func newV86Resource(le *logrus.Entry, objectKey string, ws world.WorldState, b bus.Bus, v86fsServer *unixfs_v86fs.Server) *v86Resource {
	return &v86Resource{le: le, objectKey: objectKey, ws: ws, b: b, v86fsServer: v86fsServer}
}

// acquireHomeMount keeps the run-scoped home mount registered until every
// Execute stream using the runtime releases it.
func (r *v86Resource) acquireHomeMount(ctx context.Context) (func(), error) {
	r.homeMountMtx.Lock()
	defer r.homeMountMtx.Unlock()

	if r.homeMountRefs == 0 {
		cleanup, err := ensureHomeMount(ctx, r.le, r.ws, r.objectKey, r.v86fsServer)
		if err != nil {
			return nil, err
		}
		r.homeMountCleanup = cleanup
	}
	r.homeMountRefs++

	var once sync.Once
	return func() {
		once.Do(func() {
			r.homeMountMtx.Lock()
			defer r.homeMountMtx.Unlock()

			r.homeMountRefs--
			if r.homeMountRefs != 0 {
				return
			}
			r.homeMountCleanup()
			r.homeMountCleanup = nil
		})
	}, nil
}

// Execute reconciles desired VM state with the instanced runtime plugin.
//
// It exposes generation-fenced v86fs and runtime-status routes before loading
// the plugin. The plugin reports BOOTING, READY, ERROR, or STOPPED; only the
// accepted report committed to World becomes the observed state and process
// status. Mount or plugin-load failure is committed as ERROR.
//
// Reacts to SetV86StateOp by waiting on the object's revision via WaitRev;
// every transition updates desired state and wakes the handler.
func (r *v86Resource) Execute(req *s4wave_process.ExecuteRequest, stream s4wave_process.SRPCPersistentExecutionService_ExecuteStream) error {
	ctx := stream.Context()

	var rpRef directive.Reference
	var v86fsRouteRelease func()
	var statusRouteRelease func()
	var homeMountRelease func()
	releaseRuntime := func() {
		if rpRef != nil {
			rpRef.Release()
			rpRef = nil
		}
		if statusRouteRelease != nil {
			statusRouteRelease()
			statusRouteRelease = nil
		}
		if v86fsRouteRelease != nil {
			v86fsRouteRelease()
			v86fsRouteRelease = nil
		}
		if homeMountRelease != nil {
			homeMountRelease()
			homeMountRelease = nil
		}
	}
	defer releaseRuntime()

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

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		objState, found, err := r.ws.GetObject(ctx, r.objectKey)
		if err != nil {
			return err
		}
		if !found {
			releaseRuntime()
			if err := emit(s4wave_process.ExecutionState_ExecutionState_STOPPED, ""); err != nil {
				return err
			}
			return nil
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		desired := s4wave_vm.VmState_VmState_STOPPED
		observed := s4wave_vm.VmState_VmState_STOPPED
		generation := uint64(0)
		errorMessage := ""
		runtimePluginID := defaultVmPluginID
		_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
			vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
				return &s4wave_vm.VmV86{}
			})
			if unmarshalErr != nil {
				return unmarshalErr
			}
			if vm == nil {
				return errors.New("vm-v86 block missing on object")
			}
			desired = vm.GetState()
			observed = vm.GetObservedState()
			errorMessage = vm.GetErrorMessage()
			generation = vm.GetRunGeneration()
			if pluginID := vm.GetConfig().GetRuntimePluginId(); pluginID != "" {
				runtimePluginID = pluginID
			}
			return nil
		})
		if err != nil {
			return err
		}
		switch desired {
		case s4wave_vm.VmState_VmState_STARTING:
			desired = s4wave_vm.VmState_VmState_RUNNING
		case s4wave_vm.VmState_VmState_STOPPING:
			desired = s4wave_vm.VmState_VmState_STOPPED
		case s4wave_vm.VmState_VmState_ERROR:
			desired = s4wave_vm.VmState_VmState_STOPPED
			observed = s4wave_vm.VmState_VmState_ERROR
		}
		if desired == s4wave_vm.VmState_VmState_RUNNING && generation == 0 {
			generation, err = r.ensureRuntimeGeneration(ctx)
			if err != nil {
				return err
			}
		}

		switch desired {
		case s4wave_vm.VmState_VmState_RUNNING:
			if observed == s4wave_vm.VmState_VmState_ERROR {
				releaseRuntime()
				if err := emit(s4wave_process.ExecutionState_ExecutionState_ERROR, errorMessage); err != nil {
					return err
				}
				break
			}
			if observed != s4wave_vm.VmState_VmState_STARTING &&
				observed != s4wave_vm.VmState_VmState_RUNNING {
				changed, accepted, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_STARTING, "")
				if err != nil {
					return err
				}
				if !accepted {
					continue
				}
				if changed {
					continue
				}
			}
			if rpRef == nil {
				if mountErr := r.verifyBootMounts(ctx); mountErr != nil {
					if err := emit(mapVmState(observed), ""); err != nil {
						return err
					}
					_, _, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, mountErr.Error())
					if err != nil {
						return err
					}
					continue
				}
				var mountErr error
				homeMountRelease, mountErr = r.acquireHomeMount(ctx)
				if mountErr != nil {
					if err := emit(mapVmState(observed), ""); err != nil {
						return err
					}
					_, _, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, mountErr.Error())
					if err != nil {
						return err
					}
					continue
				}

				v86fsRouteRelease, err = r.exposeV86fsToRuntimePlugin(ctx, runtimePluginID)
				if err != nil {
					releaseRuntime()
					if emitErr := emit(mapVmState(observed), ""); emitErr != nil {
						return emitErr
					}
					_, _, updateErr := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, err.Error())
					if updateErr != nil {
						return updateErr
					}
					continue
				}
				statusRouteRelease, err = r.exposeV86RuntimeStatus(ctx, runtimePluginID, generation)
				if err != nil {
					releaseRuntime()
					if emitErr := emit(mapVmState(observed), ""); emitErr != nil {
						return emitErr
					}
					_, _, updateErr := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, err.Error())
					if updateErr != nil {
						return updateErr
					}
					continue
				}

				plugin, _, newRef, loadErr := bldr_plugin.ExLoadPluginInstanced(ctx, r.b, true, runtimePluginID, r.objectKey, nil)
				if loadErr != nil || plugin == nil || newRef == nil {
					if newRef != nil {
						newRef.Release()
					}
					releaseRuntime()
					if emitErr := emit(mapVmState(observed), ""); emitErr != nil {
						return emitErr
					}
					errMsg := "v86 runtime plugin unavailable"
					if loadErr != nil {
						errMsg = loadErr.Error()
					}
					_, _, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, errMsg)
					if err != nil {
						return err
					}
					continue
				}
				rpRef = newRef
			}

			if err := emit(mapVmState(observed), ""); err != nil {
				return err
			}
		case s4wave_vm.VmState_VmState_STOPPED:
			releaseRuntime()
			if observed != s4wave_vm.VmState_VmState_STOPPED {
				changed, accepted, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_STOPPED, "")
				if err != nil {
					return err
				}
				if !accepted {
					continue
				}
				if changed {
					continue
				}
			}
			if err := emit(s4wave_process.ExecutionState_ExecutionState_STOPPED, ""); err != nil {
				return err
			}
		default:
			_, _, err := r.updateObservedState(ctx, generation, s4wave_vm.VmState_VmState_ERROR, "invalid desired v86 state")
			if err != nil {
				return err
			}
			continue
		}

		if _, err := objState.WaitRev(ctx, rev+1, false); err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			if errors.Is(err, world.ErrObjectNotFound) {
				releaseRuntime()
				return nil
			}
			return err
		}
	}
}

func (r *v86Resource) ensureRuntimeGeneration(ctx context.Context) (uint64, error) {
	objState, found, err := r.ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, world.ErrObjectNotFound
	}

	generation := uint64(0)
	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return errors.New("vm-v86 block missing on object")
		}
		generation = vm.GetRunGeneration()
		if generation == 0 {
			generation = 1
			vm.RunGeneration = generation
			bcs.SetBlock(vm, true)
		}
		return nil
	})
	return generation, err
}

type v86RuntimeStatusServer struct {
	resource   *v86Resource
	generation uint64
}

func (s *v86RuntimeStatusServer) ReportStatus(
	ctx context.Context,
	req *s4wave_vm.ReportV86RuntimeStatusRequest,
) (*s4wave_vm.ReportV86RuntimeStatusResponse, error) {
	return s.resource.applyRuntimeStatus(ctx, s.generation, req)
}

func (r *v86Resource) updateObservedState(
	ctx context.Context,
	generation uint64,
	state s4wave_vm.VmState,
	errorMessage string,
) (changed bool, accepted bool, err error) {
	objState, found, err := r.ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, world.ErrObjectNotFound
	}

	_, _, err = world.AccessObjectState(ctx, objState, true, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil || vm.GetRunGeneration() != generation {
			return nil
		}
		accepted = true
		if vm.GetObservedState() == state && vm.GetErrorMessage() == errorMessage {
			return nil
		}
		vm.ObservedState = state
		if state == s4wave_vm.VmState_VmState_ERROR {
			vm.ErrorMessage = errorMessage
		} else {
			vm.ErrorMessage = ""
		}
		bcs.SetBlock(vm, true)
		changed = true
		return nil
	})
	return changed, accepted, err
}

func (r *v86Resource) applyRuntimeStatus(
	ctx context.Context,
	generation uint64,
	req *s4wave_vm.ReportV86RuntimeStatusRequest,
) (*s4wave_vm.ReportV86RuntimeStatusResponse, error) {
	resp := &s4wave_vm.ReportV86RuntimeStatusResponse{RunGeneration: generation}
	reject := func(message string) (*s4wave_vm.ReportV86RuntimeStatusResponse, error) {
		resp.Rejection = message
		return resp, nil
	}
	if req == nil || req.GetObjectKey() != r.objectKey {
		return reject("runtime status object key does not match resource")
	}
	if req.GetRunGeneration() == 0 {
		if req.GetStatus() != s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_BOOTING {
			return reject("runtime status generation is required")
		}
	} else if req.GetRunGeneration() != generation {
		return reject("stale runtime status generation")
	}

	objState, found, err := r.ws.GetObject(ctx, r.objectKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return reject("vm object is gone")
	}
	var desired s4wave_vm.VmState
	var observed s4wave_vm.VmState
	var storedGeneration uint64
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		vm, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		if vm == nil {
			return errors.New("vm-v86 block missing on object")
		}
		desired = vm.GetState()
		observed = vm.GetObservedState()
		storedGeneration = vm.GetRunGeneration()
		return nil
	})
	if err != nil {
		return nil, err
	}
	if storedGeneration != generation {
		return reject("stale runtime status generation")
	}
	if req.GetRunGeneration() == 0 && observed != s4wave_vm.VmState_VmState_STARTING {
		return reject("bootstrap runtime status is not for a starting generation")
	}
	if desired != s4wave_vm.VmState_VmState_RUNNING &&
		req.GetStatus() != s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_STOPPED {
		return reject("runtime status is not valid for the desired state")
	}

	var state s4wave_vm.VmState
	errorMessage := ""
	switch req.GetStatus() {
	case s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_BOOTING:
		state = s4wave_vm.VmState_VmState_STARTING
	case s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_READY:
		state = s4wave_vm.VmState_VmState_RUNNING
	case s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_STOPPED:
		state = s4wave_vm.VmState_VmState_STOPPED
	case s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_ERROR:
		state = s4wave_vm.VmState_VmState_ERROR
		errorMessage = req.GetErrorMessage()
		if errorMessage == "" {
			errorMessage = "v86 runtime reported an error"
		}
	default:
		return reject("unknown runtime status")
	}
	_, accepted, err := r.updateObservedState(ctx, generation, state, errorMessage)
	if err != nil {
		return nil, err
	}
	if !accepted {
		return reject("stale runtime status generation")
	}
	resp.Accepted = true
	return resp, nil
}

func (r *v86Resource) exposeV86RuntimeStatus(
	ctx context.Context,
	pluginID string,
	generation uint64,
) (func(), error) {
	servicePrefix := v86RuntimeStatusServicePrefix + r.objectKey + "/"
	mux := srpc.NewMux(nil)
	if err := s4wave_vm.SRPCRegisterV86RuntimeStatusService(
		mux,
		&v86RuntimeStatusServer{resource: r, generation: generation},
	); err != nil {
		return nil, err
	}
	pluginServerID := bldr_plugin.PluginServerID(pluginID, "")
	workerServerID := "web-worker/" + bldr_plugin.PluginServerID(pluginID, r.objectKey)
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			"sdk/vm/world/v86/"+r.objectKey+"/runtime-status-route",
			controller.MustParseVersion("0.0.1"),
			"runtime status route for VmV86 runtime plugin",
		),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return v86fsRuntimeRouteInvoker{le: r.le, inv: mux}, nil, nil
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

func (r *v86Resource) exposeV86fsToRuntimePlugin(ctx context.Context, pluginID string) (func(), error) {
	servicePrefix := v86RuntimeV86fsServicePrefix + r.objectKey + "/"
	mux := srpc.NewMux(nil)
	if err := unixfs_v86fs.SRPCRegisterV86FsService(mux, r.v86fsServer); err != nil {
		return nil, err
	}
	pluginServerID := bldr_plugin.PluginServerID(pluginID, "")
	workerServerID := "web-worker/" + bldr_plugin.PluginServerID(pluginID, r.objectKey)
	r.le.WithFields(logrus.Fields{
		"object-key":       r.objectKey,
		"plugin-id":        pluginID,
		"service-prefix":   servicePrefix,
		"plugin-server-id": pluginServerID,
		"worker-server-id": workerServerID,
	}).Debug("v86fs runtime route exposing")
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			"sdk/vm/world/v86/"+r.objectKey+"/v86fs-runtime-route",
			controller.MustParseVersion("0.0.1"),
			"v86fs route for VmV86 runtime plugin",
		),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return v86fsRuntimeRouteInvoker{le: r.le, inv: mux}, nil, nil
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
	return func() {
		r.le.WithFields(logrus.Fields{
			"object-key":     r.objectKey,
			"service-prefix": servicePrefix,
		}).Debug("v86fs runtime route releasing")
		release()
	}, nil
}

type v86fsRuntimeRouteInvoker struct {
	le  *logrus.Entry
	inv srpc.Invoker
}

func (i v86fsRuntimeRouteInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ok, err := i.inv.InvokeMethod(serviceID, methodID, strm)
	i.le.WithError(err).
		WithField("service-id", serviceID).
		WithField("method-id", methodID).
		WithField("ok", ok).
		Debug("v86fs runtime route invoke")
	return ok, err
}

// verifyBootMounts confirms the assets required before v86 can start resolve
// through the V86Image or override edges before the plugin is loaded. Any
// failure here is treated as an ERROR state for the handler.
func (r *v86Resource) verifyBootMounts(ctx context.Context) error {
	for _, mountName := range []string{"", "wasm", "seabios", "vgabios", "kernel"} {
		fsh, err := resolveV86Mount(ctx, r.le, r.ws, r.objectKey, mountName)
		if err != nil {
			displayName := mountName
			if displayName == "" {
				displayName = "rootfs"
			}
			return errors.Wrapf(err, "verify v86 boot mount %q", displayName)
		}
		if _, err := fsh.GetNodeType(ctx); err != nil {
			displayName := mountName
			if displayName == "" {
				displayName = "rootfs"
			}
			fsh.Release()
			return errors.Wrapf(err, "verify v86 boot mount %q node type", displayName)
		}
		if _, err := fsh.GetPermissions(ctx); err != nil {
			displayName := mountName
			if displayName == "" {
				displayName = "rootfs"
			}
			fsh.Release()
			return errors.Wrapf(err, "verify v86 boot mount %q permissions", displayName)
		}
		fsh.Release()
	}
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
