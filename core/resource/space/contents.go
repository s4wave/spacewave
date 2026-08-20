package resource_space

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

type spaceContentsControllerStarter func(
	ctx context.Context,
	b bus.Bus,
	conf *plugin_space.Config,
) (*plugin_space.Controller, directive.Reference, error)

type spaceRuntime struct {
	bus              bus.Bus
	scheduler        *plugin_host_scheduler.Controller
	schedulerRelease func()
	mirrorRelease    func()
	hostWatchRelease func()
	bridgeRef        func()
	cancel           context.CancelFunc
	done             chan struct{}
	terminal         <-chan error
	releaseOnce      sync.Once
}

func (r *spaceRuntime) Release() {
	if r == nil {
		return
	}
	r.releaseOnce.Do(func() {
		if r.schedulerRelease != nil {
			r.schedulerRelease()
		}
		if r.mirrorRelease != nil {
			r.mirrorRelease()
		}
		if r.hostWatchRelease != nil {
			r.hostWatchRelease()
		}
		if r.bridgeRef != nil {
			r.bridgeRef()
		}
		if r.cancel != nil {
			r.cancel()
		}
		if r.done != nil {
			close(r.done)
		}
	})
}

type attachedRpcServiceBinding struct {
	// runtime retains the Space runtime that serves this binding.
	runtime *spaceRuntime
	// controller routes the attached resource through the Space runtime.
	controller *attachedRpcServiceController
}

// attachedRpcServiceController signals when its RPC service can resolve calls.
type attachedRpcServiceController struct {
	// RpcServiceController provides the RPC route after it receives a context.
	*bifrost_rpc.RpcServiceController
	// ready closes after RpcServiceController.Execute sets its context.
	ready chan struct{}
	// readyOnce guards ready closure when ControllerBus executes this controller.
	readyOnce sync.Once
}

// Execute initializes the RPC service controller and then signals readiness.
func (c *attachedRpcServiceController) Execute(ctx context.Context) error {
	err := c.RpcServiceController.Execute(ctx)
	c.readyOnce.Do(func() { close(c.ready) })
	return err
}

type spaceRuntimeStarter func(
	ctx context.Context,
	parent bus.Bus,
	le *logrus.Entry,
	conf *plugin_space.Config,
) (*spaceRuntime, error)

type spaceRuntimeTerminalWaiter func(
	seq uint64,
	runtime *spaceRuntime,
	ctrlRef directive.Reference,
	conf *plugin_space.Config,
)

var errSpaceRuntimePluginHostSetChanged = errors.New("daemon plugin host set changed")

// SpaceContentsResource provides streaming plugin status for a mounted space.
type SpaceContentsResource struct {
	le        *logrus.Entry
	b         bus.Bus
	mux       srpc.Invoker
	engine    world.Engine
	spaceID   string
	engineID  string
	volumeID  string
	storeID   string
	ctx       context.Context
	ctxCancel context.CancelFunc
	start     *routine.RoutineContainer
	startSeq  uint64
	released  bool
	// ctrlRef holds the plugin/space controller reference.
	// Released when the resource is cleaned up.
	ctrlRef directive.Reference
	// runtime retains the isolated scheduler and child bus for this Space.
	runtime *spaceRuntime
	// attachedRpcServices is guarded by bcast and retains one binding per private service prefix.
	attachedRpcServices map[string]*attachedRpcServiceBinding
	// startErr is the terminal error from the current runtime startup.
	startErr error
	// ctrl wakes the running plugin/space controller after content changes.
	ctrl *plugin_space.Controller
	// bcast is broadcast when content state changes so WatchState re-sends. It
	// also guards the cached plugin descriptions and manifest catalog below.
	bcast broadcast.Broadcast
	// descriptionPluginIDs is the plugin ID set for the cached descriptions.
	descriptionPluginIDs []string
	// descriptions caches plugin descriptions for the current plugin set.
	descriptions map[string]string
	// availablePluginManifestRefs fingerprints the manifest object content set for
	// the cached catalog.
	availablePluginManifestRefs []string
	// availablePlugins caches the installable plugin catalog for the current
	// manifest object content set.
	availablePlugins []*s4wave_space.AvailablePlugin
	// buildDescriptions overrides description lookup in tests.
	buildDescriptions func(context.Context, world.WorldState, []string) (map[string]string, error)
	// buildAvailablePlugins overrides catalog enumeration in tests.
	buildAvailablePlugins func(context.Context, world.WorldState) ([]*s4wave_space.AvailablePlugin, error)
	// lookupManifest overrides manifest lookup in tests.
	lookupManifest  func(context.Context, world.WorldState, string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error)
	startController spaceContentsControllerStarter
	startRuntime    spaceRuntimeStarter
	// waitRuntimeTerminal runs the current runtime's terminal lifecycle waiter.
	waitRuntimeTerminal spaceRuntimeTerminalWaiter
}

// NewSpaceContentsResource creates a new SpaceContentsResource.
func NewSpaceContentsResource(le *logrus.Entry, b bus.Bus, engine world.Engine, spaceID, engineID string) *SpaceContentsResource {
	ctx, cancel := context.WithCancel(context.Background())
	r := &SpaceContentsResource{
		le:              le,
		b:               b,
		engine:          engine,
		spaceID:         spaceID,
		engineID:        engineID,
		ctx:             ctx,
		ctxCancel:       cancel,
		start:           newSpaceContentsStartRoutine(le),
		startController: startSpaceContentsController,
		startRuntime:    startSpaceRuntime,
	}
	r.waitRuntimeTerminal = r.runRuntimeTerminalWaiter
	r.start.SetContext(ctx, false)
	mux := srpc.NewMux()
	_ = s4wave_space.SRPCRegisterSpaceContentsResourceService(mux, r)
	r.mux = mux
	return r
}

// Release releases the controller reference.
func (r *SpaceContentsResource) Release() {
	var start *routine.RoutineContainer
	var ref directive.Reference
	var cancel context.CancelFunc
	var runtime *spaceRuntime
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.released = true
		r.startSeq++
		start = r.start
		cancel = r.ctxCancel
		r.ctxCancel = nil
		r.ctx = nil
		ref = r.ctrlRef
		r.ctrlRef = nil
		r.ctrl = nil
		runtime = r.runtime
		r.runtime = nil
		broadcast()
	})
	if start != nil {
		start.ClearContext()
	}
	if ref != nil {
		ref.Release()
	}
	if runtime != nil {
		runtime.Release()
	}
	if cancel != nil {
		cancel()
	}
}

// notifyChanged signals WatchState to re-read and re-send.
func (r *SpaceContentsResource) notifyChanged() {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

func (r *SpaceContentsResource) getStoreLocation() (string, string) {
	volumeID := r.volumeID
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	storeID := r.storeID
	if storeID == "" {
		storeID = process_binding.DefaultObjectStoreID
	}
	return volumeID, storeID
}

func (r *SpaceContentsResource) notifyController() {
	var ctrl *plugin_space.Controller
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ctrl = r.ctrl
	})
	if ctrl != nil {
		ctrl.NotifyChanged()
	}
}

// GetMux returns the rpc mux.
func (r *SpaceContentsResource) GetMux() srpc.Invoker {
	return r.mux
}

// StartController starts the plugin/space controller behind this resource.
func (r *SpaceContentsResource) StartController(conf *plugin_space.Config) {
	conf = conf.CloneVT()
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.startControllerLocked(conf, broadcast)
	})
}

// startControllerLocked reserves the next controller start while bcast is held.
func (r *SpaceContentsResource) startControllerLocked(
	conf *plugin_space.Config,
	broadcast func(),
) {
	if r.released {
		return
	}
	r.ensureStartOwnerLocked()
	r.startSeq++
	seq := r.startSeq
	r.startErr = nil
	r.start.SetRoutine(func(ctx context.Context) error {
		return r.startControllerRoutine(ctx, seq, conf)
	})
	broadcast()
}

func (r *SpaceContentsResource) ensureStartOwnerLocked() {
	if r.ctx == nil {
		r.ctx, r.ctxCancel = context.WithCancel(context.Background())
	}
	if r.start == nil {
		r.start = newSpaceContentsStartRoutine(r.le)
		r.start.SetContext(r.ctx, false)
	}
	if r.startController == nil {
		r.startController = startSpaceContentsController
	}
	if r.startRuntime == nil {
		r.startRuntime = startSpaceRuntime
	}
	if r.waitRuntimeTerminal == nil {
		r.waitRuntimeTerminal = r.runRuntimeTerminalWaiter
	}
}

func (r *SpaceContentsResource) startControllerRoutine(ctx context.Context, seq uint64, conf *plugin_space.Config) error {
	var runtime *spaceRuntime
	var ctrl *plugin_space.Controller
	var ctrlRef directive.Reference
	var err error
	if r.startRuntime != nil {
		runtime, err = r.startRuntime(ctx, r.b, r.le, conf)
		if err == nil {
			ctrl, ctrlRef, err = r.startController(ctx, runtime.bus, conf)
		}
	} else {
		ctrl, ctrlRef, err = r.startController(ctx, r.b, conf)
	}
	if err != nil {
		if runtime != nil {
			runtime.Release()
		}
		if ctx.Err() == nil && r.le != nil {
			r.le.WithError(err).Warn("failed to start Space contents controller")
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if !r.released && r.startSeq == seq && ctx.Err() == nil {
				r.startErr = err
				broadcast()
			}
		})
		return nil
	}

	var installed bool
	var release directive.Reference
	var releaseRuntime *spaceRuntime
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.released || r.startSeq != seq || ctx.Err() != nil {
			release = ctrlRef
			releaseRuntime = runtime
		} else {
			release = r.ctrlRef
			releaseRuntime = r.runtime
			r.ctrl = ctrl
			r.ctrlRef = ctrlRef
			r.runtime = runtime
			r.startErr = nil
			installed = true
		}
		broadcast()
	})
	if release != nil {
		release.Release()
	}
	if releaseRuntime != nil {
		releaseRuntime.Release()
	}
	if runtime != nil && installed {
		go r.waitRuntimeTerminal(seq, runtime, ctrlRef, conf)
	}
	return nil
}

func (r *SpaceContentsResource) runRuntimeTerminalWaiter(
	seq uint64,
	runtime *spaceRuntime,
	ctrlRef directive.Reference,
	conf *plugin_space.Config,
) {
	var err error
	var ok bool
	select {
	case <-runtime.done:
		return
	case err, ok = <-runtime.terminal:
	}
	if !ok || err == nil {
		return
	}
	var release bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.released || r.startSeq != seq || r.runtime != runtime {
			return
		}
		r.ctrl = nil
		r.ctrlRef = nil
		r.runtime = nil
		release = true
		if errors.Is(err, errSpaceRuntimePluginHostSetChanged) {
			r.startControllerLocked(conf.CloneVT(), broadcast)
			return
		}
		r.startErr = err
		broadcast()
	})
	if release {
		ctrlRef.Release()
		runtime.Release()
	}
}

func newSpaceContentsStartRoutine(le *logrus.Entry) *routine.RoutineContainer {
	if le == nil {
		return routine.NewRoutineContainer()
	}
	return routine.NewRoutineContainerWithLogger(le.WithField("routine", "space-contents-start"))
}

func startSpaceRuntime(
	ctx context.Context,
	parent bus.Bus,
	le *logrus.Entry,
	conf *plugin_space.Config,
) (*spaceRuntime, error) {
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	childCtx, childCancel := context.WithCancel(context.WithoutCancel(ctx))
	child, resolver, err := controllerbus_core.NewCoreBus(childCtx, le)
	if err != nil {
		childCancel()
		return nil, err
	}
	resolver.AddFactory(plugin_host_scheduler.NewFactory(child))
	resolver.AddFactory(plugin_space.NewFactory(child, plugin_space.WithManifestSource(parent)))
	bridgeCtrl := bus_bridge.NewBusBridge(parent, spaceRuntimeBridgeFilter)
	bridgeRef, err := child.AddController(childCtx, bridgeCtrl, nil)
	if err != nil {
		childCancel()
		return nil, err
	}

	mirror := newSpacePluginHostMirror()
	mirrorRelease, err := child.AddController(childCtx, mirror, nil)
	if err != nil {
		bridgeRef()
		childCancel()
		return nil, err
	}
	hostReady := make(chan error, 1)
	terminal := make(chan error, 1)
	var initialSnapshot atomic.Bool
	reportTerminal := func(err error) {
		select {
		case terminal <- err:
			childCancel()
		default:
		}
	}
	_, hostWatchRelease, err := bus.ExecCollectValuesWatch(
		childCtx,
		parent,
		plugin_host.NewLookupPluginHost(nil),
		true,
		func(resErr []error, hosts []plugin_host.PluginHost) error {
			if len(resErr) != 0 {
				reportTerminal(fmt.Errorf("watch daemon plugin hosts: %w", resErr[0]))
				if !initialSnapshot.Load() {
					hostReady <- resErr[0]
				}
				return nil
			}
			if !initialSnapshot.CompareAndSwap(false, true) {
				reportTerminal(errSpaceRuntimePluginHostSetChanged)
				return nil
			}
			mirror.SetHosts(hosts)
			hostReady <- nil
			return nil
		},
		func(err error) {
			if initialSnapshot.Load() {
				reportTerminal(fmt.Errorf("watch daemon plugin hosts: %w", err))
				return
			}
			hostReady <- err
		},
	)
	if err != nil {
		mirrorRelease()
		bridgeRef()
		childCancel()
		return nil, err
	}
	select {
	case err = <-hostReady:
	case <-ctx.Done():
		err = context.Canceled
	}
	if err != nil {
		hostWatchRelease()
		mirrorRelease()
		bridgeRef()
		childCancel()
		return nil, err
	}

	scheduler, schedulerRelease, err := plugin_host_default.StartNativeDesktopPluginScheduler(
		childCtx,
		child,
		conf.GetSpaceId(),
		conf.GetEngineId(),
		bldr_plugin.PluginVolumeID,
		bldr_plugin.PluginVolumeID,
		conf.GetSessionPeerId(),
		true,
		true,
		true,
		[]string{},
	)
	if err != nil {
		hostWatchRelease()
		mirrorRelease()
		bridgeRef()
		childCancel()
		return nil, err
	}
	return &spaceRuntime{
		bus:              child,
		scheduler:        scheduler,
		schedulerRelease: schedulerRelease,
		mirrorRelease:    mirrorRelease,
		hostWatchRelease: hostWatchRelease,
		bridgeRef:        bridgeRef,
		cancel:           childCancel,
		done:             make(chan struct{}),
		terminal:         terminal,
	}, nil
}

func spaceRuntimeBridgeFilter(inst directive.Instance) (bool, error) {
	return spaceRuntimeBridgeDirective(inst.GetDirective()), nil
}

func spaceRuntimeBridgeDirective(dir directive.Directive) bool {
	switch dir.(type) {
	case world.LookupWorldEngine, world.LookupWorldOp,
		volume.LookupVolume, volume.BuildObjectStoreAPI,
		plugin_host_root.LookupRoot:
		return true
	default:
		return false
	}
}

func startSpaceContentsController(
	ctx context.Context,
	b bus.Bus,
	conf *plugin_space.Config,
) (*plugin_space.Controller, directive.Reference, error) {
	ctrl, _, ctrlRef, err := plugin_space.StartControllerWithConfig(ctx, b, conf, func() {})
	return ctrl, ctrlRef, err
}

// BindAttachedRpcService publishes a caller-attached Resource under one private
// service ID prefix until the caller disconnects or this Space runtime ends.
func (r *SpaceContentsResource) BindAttachedRpcService(
	req *s4wave_space.BindAttachedRpcServiceRequest,
	strm s4wave_space.SRPCSpaceContentsResourceService_BindAttachedRpcServiceStream,
) error {
	if req.GetAttachedResourceId() == 0 {
		return errors.New("attached resource ID must be nonzero")
	}
	prefix := req.GetServiceIdPrefix()
	if !isSafeAttachedRpcServicePrefix(prefix) {
		return errors.New("service ID prefix must be nonempty and safe")
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(strm.Context())
	if err != nil {
		return err
	}
	client, err := resourceCtx.GetAttachedResource(req.GetAttachedResourceId())
	if err != nil {
		return err
	}
	var clientDone <-chan struct{}
	if doneClient, ok := client.(interface{ Done() <-chan struct{} }); ok {
		clientDone = doneClient.Done()
	}

	// Refuse a route when the caller, attachment, or request has already ended.
	if err := attachedRpcServiceLifetimeError(strm.Context(), resourceCtx.Context(), clientDone, nil); err != nil {
		return err
	}

	ctrl := &attachedRpcServiceController{
		RpcServiceController: bifrost_rpc.NewRpcServiceController(
			controller.NewInfo(
				"core/resource/space/attached-rpc-service/"+prefix,
				controller.MustParseVersion("0.0.1"),
				"attached RPC service route",
			),
			bifrost_rpc.NewRpcServiceBuilder(srpc.NewClientInvoker(client)),
			[]string{prefix},
			true,
			nil,
			nil,
			nil,
		),
		ready: make(chan struct{}),
	}
	binding := &attachedRpcServiceBinding{controller: ctrl}

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if r.runtime == nil {
			err = errors.New("space runtime is not running")
			return
		}
		if r.attachedRpcServices == nil {
			r.attachedRpcServices = make(map[string]*attachedRpcServiceBinding)
		}
		if existing := r.attachedRpcServices[prefix]; existing != nil {
			select {
			case <-existing.runtime.done:
				delete(r.attachedRpcServices, prefix)
			default:
				err = fmt.Errorf("service ID prefix %q is already bound", prefix)
				return
			}
		}
		binding.runtime = r.runtime
		r.attachedRpcServices[prefix] = binding
	})
	if err != nil {
		return err
	}

	// Check the installed binding lifetime before ControllerBus starts its route.
	if err := attachedRpcServiceLifetimeError(strm.Context(), resourceCtx.Context(), clientDone, binding.runtime.done); err != nil {
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if r.attachedRpcServices[prefix] == binding {
				delete(r.attachedRpcServices, prefix)
			}
		})
		return err
	}

	release, err := binding.runtime.bus.AddController(strm.Context(), binding.controller, nil)
	if err != nil {
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if r.attachedRpcServices[prefix] == binding {
				delete(r.attachedRpcServices, prefix)
			}
		})
		return err
	}
	defer func() {
		release()
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if r.attachedRpcServices[prefix] == binding {
				delete(r.attachedRpcServices, prefix)
			}
		})
	}()

	select {
	case <-binding.controller.ready:
	case <-strm.Context().Done():
		return strm.Context().Err()
	case <-resourceCtx.Context().Done():
		return resourceCtx.Context().Err()
	case <-clientDone:
		return errors.New("attached resource ended before attached service was ready")
	case <-binding.runtime.done:
		return errors.New("space runtime ended before attached service was ready")
	}

	// Give an already-ended lifetime priority over the ready notification.
	if err := attachedRpcServiceLifetimeError(strm.Context(), resourceCtx.Context(), clientDone, binding.runtime.done); err != nil {
		return err
	}
	if err := strm.Send(&s4wave_space.BindAttachedRpcServiceResponse{}); err != nil {
		return err
	}
	select {
	case <-strm.Context().Done():
		return strm.Context().Err()
	case <-resourceCtx.Context().Done():
		return resourceCtx.Context().Err()
	case <-clientDone:
		return nil
	case <-binding.runtime.done:
		return nil
	}
}

// attachedRpcServiceLifetimeError reports an ended binding lifetime without waiting.
func attachedRpcServiceLifetimeError(
	streamCtx context.Context,
	resourceCtx context.Context,
	clientDone <-chan struct{},
	runtimeDone <-chan struct{},
) error {
	if err := streamCtx.Err(); err != nil {
		return err
	}
	if err := resourceCtx.Err(); err != nil {
		return err
	}
	select {
	case <-clientDone:
		return errors.New("attached resource ended before attached service was ready")
	default:
	}
	select {
	case <-runtimeDone:
		return errors.New("space runtime ended before attached service was ready")
	default:
	}
	return nil
}

func isSafeAttachedRpcServicePrefix(prefix string) bool {
	if prefix == "" || len(prefix) > 256 || !utf8.ValidString(prefix) ||
		prefix[0] == '/' || prefix[len(prefix)-1] != '/' {
		return false
	}
	for _, char := range prefix {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return false
		}
	}
	return true
}

// WatchState streams the current plugin and process state for the space.
func (r *SpaceContentsResource) WatchState(
	req *s4wave_space.WatchSpaceContentsStateRequest,
	strm s4wave_space.SRPCSpaceContentsResourceService_WatchStateStream,
) error {
	ctx := strm.Context()

	var prevSeqno uint64
	for {
		// Read SpaceSettings, manifest descriptions, and the installable plugin
		// catalog from the world.
		var pluginIDs []string
		var descriptions map[string]string
		var availablePlugins []*s4wave_space.AvailablePlugin
		if err := func() error {
			wtx, err := r.engine.NewTransaction(ctx, false)
			if err != nil {
				return err
			}
			defer wtx.Discard()

			prevSeqno, err = wtx.GetSeqno(ctx)
			if err != nil {
				return err
			}

			settings, _, err := space_world.LookupSpaceSettings(ctx, wtx)
			if err != nil {
				return err
			}
			if settings != nil {
				pluginIDs = settings.GetPluginIds()
			}

			descriptions, err = r.getPluginDescriptions(ctx, wtx, pluginIDs)
			if err != nil {
				r.le.WithError(err).Warn("failed to resolve plugin descriptions")
				descriptions = nil
			}

			availablePlugins, err = r.getAvailablePlugins(ctx, wtx)
			if err != nil {
				r.le.WithError(err).Warn("failed to resolve plugin catalog")
				availablePlugins = nil
			}

			return nil
		}(); err != nil {
			return err
		}

		// Build plugin statuses.
		plugins := make([]*s4wave_space.SpacePluginStatus, 0, len(pluginIDs))
		loadedIDs := map[string]struct{}{}
		var loadedCh <-chan struct{}
		var waitStatusChange func(context.Context) error
		var ctrl *plugin_space.Controller
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			ctrl = r.ctrl
		})
		if ctrl != nil {
			var ids []string
			ids, loadedCh = ctrl.GetLoadedPluginIDsAndWaitCh()
			for _, pid := range ids {
				loadedIDs[pid] = struct{}{}
			}
		}
		var startErr error
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			startErr = r.startErr
		})
		schedulerStatuses := map[string]*bldr_plugin.PluginStatus{}
		var scheduler *plugin_host_scheduler.Controller
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if r.runtime != nil {
				scheduler = r.runtime.scheduler
			}
		})
		if scheduler == nil {
			scheduler = plugin_host_scheduler.FindControllerOnBus(r.b)
		}
		if scheduler != nil {
			statusCtr := scheduler.GetPluginStatusCtr()
			statusSnapshot := statusCtr.GetValue()
			schedulerStatuses = spacePluginStatusesByID(statusSnapshot, r.spaceID)
			waitStatusChange = func(waitCtx context.Context) error {
				_, err := statusCtr.WaitValueChange(waitCtx, statusSnapshot, nil)
				return err
			}
		}
		for _, pid := range pluginIDs {
			_, loaded := loadedIDs[pid]
			status := buildSpacePluginStatus(
				pid,
				descriptions[pid],
				loaded,
				ctrl != nil,
				schedulerStatuses[pid],
			)
			if startErr != nil {
				status.State = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED
				status.Detail = startErr.Error()
			}
			plugins = append(plugins, status)
		}
		processBindings, err := r.listProcessBindingInfos(ctx)
		if err != nil {
			return err
		}

		if err := strm.Send(&s4wave_space.SpaceContentsState{
			Ready:            true,
			Plugins:          plugins,
			ProcessBindings:  processBindings,
			AvailablePlugins: availablePlugins,
		}); err != nil {
			return err
		}

		// Wait for world seqno change, process-binding change, or loaded state change.
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		err = waitSpaceContentsSources(ctx, func(waitCtx context.Context) error {
			_, err := r.engine.WaitSeqno(waitCtx, prevSeqno+1)
			return err
		}, []<-chan struct{}{ch, loadedCh}, waitStatusChange)
		if err != nil {
			return err
		}
	}
}

func waitSpaceContentsSeqno(
	ctx context.Context,
	waitSeqno func(context.Context) error,
	waitChs ...<-chan struct{},
) error {
	return waitSpaceContentsSources(ctx, waitSeqno, waitChs, nil)
}

func waitSpaceContentsSources(
	ctx context.Context,
	waitSeqno func(context.Context) error,
	waitChs []<-chan struct{},
	waitFns ...func(context.Context) error,
) error {
	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitCount := 1
	for _, waitFn := range waitFns {
		if waitFn != nil {
			waitCount++
		}
	}
	waitAnyDone := make(chan struct{}, waitCount)
	go func() {
		if err := broadcast.WaitAny(waitCtx, waitChs...); err == nil {
			waitCancel()
		}
		waitAnyDone <- struct{}{}
	}()
	for _, waitFn := range waitFns {
		if waitFn == nil {
			continue
		}
		go func() {
			if err := waitFn(waitCtx); err == nil {
				waitCancel()
			}
			waitAnyDone <- struct{}{}
		}()
	}

	err := waitSeqno(waitCtx)
	waitCancel()
	for range waitCount {
		<-waitAnyDone
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func spacePluginStatusesByID(
	snapshot *plugin_host_scheduler.PluginStatusSnapshot,
	instanceKey string,
) map[string]*bldr_plugin.PluginStatus {
	statuses := map[string]*bldr_plugin.PluginStatus{}
	if snapshot == nil {
		return statuses
	}
	for _, plugin := range snapshot.Plugins {
		if plugin == nil || plugin.GetInstanceKey() != instanceKey {
			continue
		}
		statuses[plugin.GetPluginId()] = plugin
	}
	return statuses
}

func buildSpacePluginStatus(
	pluginID string,
	description string,
	loaded bool,
	controllerStarted bool,
	schedulerStatus *bldr_plugin.PluginStatus,
) *s4wave_space.SpacePluginStatus {
	state := s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_CONFIGURED
	detail := ""
	if controllerStarted {
		state = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING
		detail = "Plugin runtime requested"
	}
	if schedulerStatus != nil {
		state, detail = projectSpacePluginLifecycle(schedulerStatus)
	}
	if loaded || (schedulerStatus != nil && schedulerStatus.GetRunning()) {
		loaded = true
		state = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED
		detail = ""
	}
	return &s4wave_space.SpacePluginStatus{
		PluginId:    pluginID,
		Loaded:      loaded,
		Description: description,
		State:       state,
		Detail:      detail,
	}
}

func projectSpacePluginLifecycle(
	status *bldr_plugin.PluginStatus,
) (s4wave_space.SpacePluginLifecycleState, string) {
	if msg := status.GetLastErrorMessage(); msg != "" {
		if status.GetState() == bldr_plugin.PluginState_PluginState_REQUESTED {
			return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_RETRYING, msg
		}
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED, msg
	}
	switch status.GetState() {
	case bldr_plugin.PluginState_PluginState_RUNNING:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED, ""
	case bldr_plugin.PluginState_PluginState_REQUESTED:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING, "Plugin runtime requested"
	default:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_CONFIGURED, ""
	}
}

// getPluginDescriptions returns cached plugin descriptions for the current plugin set.
func (r *SpaceContentsResource) getPluginDescriptions(
	ctx context.Context,
	ws world.WorldState,
	pluginIDs []string,
) (map[string]string, error) {
	pluginIDs = slices.Clone(pluginIDs)

	var cached map[string]string
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if slices.Equal(r.descriptionPluginIDs, pluginIDs) {
			cached = maps.Clone(r.descriptions)
		}
	})
	if cached != nil {
		return cached, nil
	}

	buildDescriptions := r.buildDescriptions
	if buildDescriptions == nil {
		buildDescriptions = r.collectPluginDescriptions
	}
	descriptions, err := buildDescriptions(ctx, ws, pluginIDs)
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.descriptionPluginIDs = slices.Clone(pluginIDs)
		r.descriptions = maps.Clone(descriptions)
	})
	return maps.Clone(descriptions), nil
}

// collectPluginDescriptions builds a description summary for the current plugin set.
func (r *SpaceContentsResource) collectPluginDescriptions(
	ctx context.Context,
	ws world.WorldState,
	pluginIDs []string,
) (map[string]string, error) {
	descriptions := make(map[string]string, len(pluginIDs))
	if len(pluginIDs) == 0 {
		return descriptions, nil
	}

	needed := make(map[string]struct{}, len(pluginIDs))
	for _, pid := range pluginIDs {
		if pid != "" {
			needed[pid] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return descriptions, nil
	}

	manifestKeys, err := world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		return nil, err
	}
	for _, key := range manifestKeys {
		m, _, err := bldr_manifest_world.LookupManifest(ctx, ws, key)
		if err != nil {
			continue
		}
		meta := m.GetMeta()
		mid := meta.GetManifestId()
		if _, ok := needed[mid]; !ok {
			continue
		}
		if _, ok := descriptions[mid]; ok {
			continue
		}
		if desc := meta.GetDescription(); desc != "" {
			descriptions[mid] = desc
		}
		if len(descriptions) == len(needed) {
			break
		}
	}

	return descriptions, nil
}

// getAvailablePlugins returns the cached installable plugin catalog for the
// current manifest object content set, using the test override when set.
func (r *SpaceContentsResource) getAvailablePlugins(
	ctx context.Context,
	ws world.WorldState,
) ([]*s4wave_space.AvailablePlugin, error) {
	if r.buildAvailablePlugins != nil {
		return r.buildAvailablePlugins(ctx, ws)
	}

	manifestRefs, err := collectAvailablePluginManifestRefs(ctx, ws)
	if err != nil {
		return nil, err
	}

	var cached []*s4wave_space.AvailablePlugin
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if slices.Equal(r.availablePluginManifestRefs, manifestRefs) {
			cached = cloneAvailablePlugins(r.availablePlugins)
		}
	})
	if cached != nil {
		return cached, nil
	}

	availablePlugins, err := r.collectAvailablePlugins(ctx, ws, manifestRefs)
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.availablePluginManifestRefs = slices.Clone(manifestRefs)
		r.availablePlugins = cloneAvailablePlugins(availablePlugins)
	})
	return cloneAvailablePlugins(availablePlugins), nil
}

func collectAvailablePluginManifestRefs(
	ctx context.Context,
	ws world.WorldState,
) ([]string, error) {
	manifestKeys, err := world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		return nil, err
	}
	manifestKeys = slices.Clone(manifestKeys)
	slices.Sort(manifestKeys)

	manifestRefs := make([]string, 0, len(manifestKeys))
	for _, key := range manifestKeys {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		ref, _, err := obj.GetRootRef(ctx)
		if err != nil {
			return nil, err
		}
		manifestRefs = append(manifestRefs, key+"\x00"+ref.MarshalString())
	}
	return manifestRefs, nil
}

// collectAvailablePlugins enumerates the manifest object content set and returns
// the installable plugin catalog, keeping the highest revision for each manifest
// ID.
func (r *SpaceContentsResource) collectAvailablePlugins(
	ctx context.Context,
	ws world.WorldState,
	manifestRefs []string,
) ([]*s4wave_space.AvailablePlugin, error) {
	lookupManifest := r.lookupManifest
	if lookupManifest == nil {
		lookupManifest = bldr_manifest_world.LookupManifest
	}

	catalog := make(map[string]*bldr_manifest.ManifestMeta, len(manifestRefs))
	for _, ref := range manifestRefs {
		key, _, _ := strings.Cut(ref, "\x00")
		m, _, err := lookupManifest(ctx, ws, key)
		if err != nil || m == nil {
			continue
		}
		addManifestToCatalog(catalog, m.GetMeta())
	}

	return availablePluginsFromCatalog(catalog), nil
}

// addManifestToCatalog records the manifest meta under its manifest ID, keeping
// the highest revision when the same plugin has multiple platform/revision
// builds.
func addManifestToCatalog(catalog map[string]*bldr_manifest.ManifestMeta, meta *bldr_manifest.ManifestMeta) {
	manifestID := meta.GetManifestId()
	if manifestID == "" {
		return
	}
	if prev, ok := catalog[manifestID]; !ok || meta.GetRev() > prev.GetRev() {
		catalog[manifestID] = meta
	}
}

// availablePluginsFromCatalog projects the collected catalog into the sorted
// app-facing available plugin list.
func availablePluginsFromCatalog(catalog map[string]*bldr_manifest.ManifestMeta) []*s4wave_space.AvailablePlugin {
	out := make([]*s4wave_space.AvailablePlugin, 0, len(catalog))
	for manifestID, meta := range catalog {
		out = append(out, &s4wave_space.AvailablePlugin{
			PluginId:    manifestID,
			Description: meta.GetDescription(),
			Revision:    strconv.FormatUint(meta.GetRev(), 10),
		})
	}
	slices.SortFunc(out, func(a, b *s4wave_space.AvailablePlugin) int {
		return cmp.Compare(a.GetPluginId(), b.GetPluginId())
	})
	return out
}

func cloneAvailablePlugins(in []*s4wave_space.AvailablePlugin) []*s4wave_space.AvailablePlugin {
	if in == nil {
		return nil
	}
	out := make([]*s4wave_space.AvailablePlugin, 0, len(in))
	for _, plugin := range in {
		out = append(out, plugin.CloneVT())
	}
	return out
}

// SetProcessBinding sets the state for a process binding.
func (r *SpaceContentsResource) SetProcessBinding(
	ctx context.Context,
	req *s4wave_space.SetProcessBindingRequest,
) (*s4wave_space.SetProcessBindingResponse, error) {
	objKey := req.GetObjectKey()
	if objKey == "" {
		return nil, errors.New("object_key is required")
	}
	typeID := req.GetTypeId()
	if typeID == "" {
		return nil, errors.New("type_id is required")
	}

	volumeID, storeID := r.getStoreLocation()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		r.b,
		true,
		storeID,
		volumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	state := s4wave_process.ProcessBindingState_ProcessBindingState_UNAPPROVED
	if req.GetApproved() {
		state = s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED
	}

	binding := &s4wave_process.ProcessBinding{
		State:     state,
		ObjectKey: objKey,
		TypeId:    typeID,
		DecidedAt: timestamppb.Now(),
	}
	if err := process_binding.SetProcessBinding(ctx, handle.GetObjectStore(), r.spaceID, objKey, binding); err != nil {
		return nil, err
	}

	r.notifyChanged()
	r.notifyController()
	return &s4wave_space.SetProcessBindingResponse{}, nil
}

func (r *SpaceContentsResource) listProcessBindingInfos(
	ctx context.Context,
) ([]*s4wave_space.ProcessBindingInfo, error) {
	volumeID, storeID := r.getStoreLocation()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		r.b,
		true,
		storeID,
		volumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	bindings, err := process_binding.ListProcessBindings(ctx, handle.GetObjectStore(), r.spaceID)
	if err != nil {
		return nil, err
	}

	infos := make([]*s4wave_space.ProcessBindingInfo, 0, len(bindings))
	for _, b := range bindings {
		infos = append(infos, &s4wave_space.ProcessBindingInfo{
			ObjectKey: b.GetObjectKey(),
			TypeId:    b.GetTypeId(),
			Approved:  b.GetState() == s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
			DecidedAt: b.GetDecidedAt(),
		})
	}

	return infos, nil
}

// _ is a type assertion
var _ s4wave_space.SRPCSpaceContentsResourceServiceServer = (*SpaceContentsResource)(nil)
