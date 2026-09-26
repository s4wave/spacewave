package plugin_space

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_list "github.com/s4wave/spacewave/core/plugin/list"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	"github.com/s4wave/spacewave/core/plugin/space/loadedplugins"
	"github.com/s4wave/spacewave/core/plugin/space/pluginids"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_objecttypes "github.com/s4wave/spacewave/core/space/world/objecttypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "plugin/space"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "loads Space plugins and resolves FetchManifest for a Space"

// processConfig is the process an approved binding runs for one object key.
type processConfig struct {
	// typeID is the ObjectType whose factory runs the process.
	typeID string
	// ws is the Space World the process reads.
	ws world.WorldState
}

// pluginReference holds one LoadPlugin directive the Space keeps loaded.
type pluginReference struct {
	// ref is the LoadPlugin directive reference.
	ref directive.Reference
	// releaseState stops watching the plugin state.
	releaseState func()
	// manifestKeys are the installed manifest keys the load selected.
	manifestKeys []string
	// releasePrevious releases the load this one replaced, once.
	releasePrevious func()
}

// release releases the load and any load it replaced.
func (r pluginReference) release() {
	r.releaseState()
	r.ref.Release()
	r.releasePrevious()
}

// processRetryBackoff paces restarts of a failed process.
var processRetryBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 1000,
		MaxInterval:     30000,
		Multiplier:      2,
	},
}

// Controller loads plugins for a Space and resolves FetchManifest directives by
// watching the Space world.
//
// Watches SpaceSettings in the Space world reactively. When plugin_ids change
// in SpaceSettings, reconciles LoadPlugin directives: adds directives for new
// plugins and releases directives for removed plugins.
//
// For FetchManifest: resolves FetchManifest directives for manifest IDs
// matching the current SpaceSettings plugin_ids. Uses a shared world watch
// loop with broadcast to handle resolver set changes. With a manifest source,
// approved requests also resolve from the parent bus.
//
// Also reconciles process bindings: starts enabled persistent processes, stops
// processes that are removed or disabled, and deletes the bindings of deleted
// World objects.
type Controller struct {
	*bus.BusController[*Config]

	// bcast guards resolvers and pluginIDs.
	bcast broadcast.Broadcast
	// resolverBcast fires when the resolver set changes.
	resolverBcast broadcast.Broadcast
	// manifestSource resolves approved manifests from the parent bus.
	manifestSource bus.Bus
	// loadTarget receives approved LoadPlugin directives when Space runs in a plugin.
	loadTarget bus.Bus
	// bindingsChanged is called after the controller deletes a process binding.
	bindingsChanged func()
	// resolvers is the set of active FetchManifest resolvers.
	resolvers map[*resolverEntry]struct{}
	// pluginIDs is the current set of plugin IDs from SpaceSettings.
	// Updated each world watch cycle. Protected by bcast.
	pluginIDs []string
	// loadedPlugins tracks demanded plugin startup readiness.
	loadedPlugins loadedplugins.State
	// processConfigs tracks the current enabled process configuration by object key.
	processConfigs map[string]processConfig
	// processes tracks active process routines by object key.
	processes *keyed.Keyed[string, processConfig]
	// watchLoop is the active world watch loop while Execute is running.
	watchLoop *world_control.WatchLoop
}

// NotifyChanged wakes the watch loop to reconcile external state changes.
func (c *Controller) NotifyChanged() {
	var watchLoop *world_control.WatchLoop
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		watchLoop = c.watchLoop
	})
	if watchLoop != nil {
		watchLoop.Wake()
	}
}

// GetLoadedPluginIDsAndWaitCh returns loaded plugin IDs and a channel closed when they change.
func (c *Controller) GetLoadedPluginIDsAndWaitCh() ([]string, <-chan struct{}) {
	return c.loadedPlugins.GetAndWaitCh()
}

// GetRequestedPluginIDsAndWaitCh returns the sorted manifest IDs with a live
// FetchManifest in the Space that no resolver has answered, and a channel
// closed when that set changes. A requested plugin that SpaceSettings does not
// list waits until it is listed. Manifests the parent supplies resolve and are
// not reported.
func (c *Controller) GetRequestedPluginIDsAndWaitCh() ([]string, <-chan struct{}) {
	var ids []string
	var waitCh <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		for entry := range c.resolvers {
			if entry.resolved {
				continue
			}
			if mid := entry.dir.GetManifestId(); !slices.Contains(ids, mid) {
				ids = append(ids, mid)
			}
		}
		waitCh = getWaitCh()
	})
	slices.Sort(ids)
	return ids, waitCh
}

// FactoryOption configures a Space plugin controller factory.
type FactoryOption func(*factoryConfig)

// factoryConfig holds the options applied by FactoryOption.
type factoryConfig struct {
	manifestSource  bus.Bus
	loadTarget      bus.Bus
	bindingsChanged func()
}

// WithManifestSource permits approved Space plugins to fetch manifests from
// source in addition to the Space World.
func WithManifestSource(source bus.Bus) FactoryOption {
	return func(conf *factoryConfig) { conf.manifestSource = source }
}

// WithLoadTarget sends approved Space plugin load directives to target.
func WithLoadTarget(target bus.Bus) FactoryOption {
	return func(conf *factoryConfig) { conf.loadTarget = target }
}

// WithProcessBindingsChanged calls notify after the controller deletes the
// process binding of a deleted World object, so binding views can refresh.
func WithProcessBindingsChanged(notify func()) FactoryOption {
	return func(conf *factoryConfig) { conf.bindingsChanged = notify }
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus, opts ...FactoryOption) controller.Factory {
	factoryConf := factoryConfig{}
	for _, opt := range opts {
		opt(&factoryConf)
	}
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			c := &Controller{
				BusController:   base,
				manifestSource:  factoryConf.manifestSource,
				loadTarget:      factoryConf.loadTarget,
				bindingsChanged: factoryConf.bindingsChanged,
				resolvers:       make(map[*resolverEntry]struct{}),
				processConfigs:  make(map[string]processConfig),
			}
			c.processes = keyed.NewKeyedWithLogger(
				c.buildProcessRoutine,
				base.GetLogger().WithField("subsystem", "process"),
				keyed.WithRetry[string, processConfig](processRetryBackoff),
			)
			return c, nil
		},
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	conf := c.GetConfig()
	engineID := conf.GetEngineId()
	if engineID == "" {
		return nil
	}
	objectTypeCtrl := objecttype_controller.NewController(space_world_objecttypes.LookupObjectType)
	objectTypeRef, err := c.GetBus().AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		return err
	}
	defer objectTypeRef()

	if conf.GetWorldBucketId() != "" {
		if conf.GetHostPluginId() == "" {
			return errors.New("host_plugin_id is required when world_bucket_id is set")
		}
		// The host connection and mounted bucket belong to the same bus that
		// receives plugin loads. Keep their RPC service on that bus as well.
		forwardBus := c.GetBus()
		if c.loadTarget != nil {
			forwardBus = c.loadTarget
		}
		engine := world.NewBusEngine(ctx, c.GetBus(), engineID)
		defer engine.ClearContext()
		forwarder := NewCloudBlockStoreForwarder(
			c.GetLogger().WithField("subsystem", "cloud-block-store-forwarding"),
			forwardBus,
			conf.GetSpaceId(),
			conf.GetWorldBucketId(),
			conf.GetHostPluginId(),
			engine.AccessWorldState,
		)
		forwarderRef, err := forwardBus.AddController(ctx, forwarder, nil)
		if err != nil {
			return errors.Wrap(err, "start cloud block store forwarder")
		}
		defer forwarderRef()
	}
	return c.runWorldWatchLoop(ctx, engineID)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case manifest.FetchManifest:
		return c.resolveFetchManifest(ctx, di, dir)
	case plugin_list.ListAvailablePlugins:
		return c.resolveListAvailablePlugins(ctx, di, dir)
	case objecttype.LookupObjectType:
		return c.resolveLookupObjectType(dir)
	}
	return nil, nil
}

// resolveListAvailablePlugins handles a ListAvailablePlugins directive.
func (c *Controller) resolveListAvailablePlugins(
	_ context.Context,
	_ directive.Instance,
	dir plugin_list.ListAvailablePlugins,
) ([]directive.Resolver, error) {
	conf := c.GetConfig()
	if dir.ListAvailablePluginsSpaceID() != conf.GetSpaceId() {
		return nil, nil
	}
	// Use current SpaceSettings plugin_ids.
	var ids []string
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ids = slices.Clone(c.pluginIDs)
	})
	return directive.R(
		plugin_list.NewResolver(c.GetBus(), dir, ids, conf.GetVolumeId(), conf.GetObjectStoreId()),
		nil,
	)
}

// resolveLookupObjectType holds LookupObjectType for this Space's engine
// non-idle while a desired plugin is still registering its object types.
func (c *Controller) resolveLookupObjectType(
	dir objecttype.LookupObjectType,
) ([]directive.Resolver, error) {
	engineID := dir.LookupObjectTypeEngineID()
	if engineID == "" || engineID != c.GetConfig().GetEngineId() {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		for {
			pending, waitCh := c.loadedPlugins.HasPendingAndWaitCh()
			handler.MarkIdle(!pending)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
			}
		}
	}), nil)
}

// resolveFetchManifest handles a FetchManifest directive. The Space World
// resolver always runs; processResolvers resolves it from World state. With a
// manifest source, a second resolver relays approved requests to the parent
// bus, so a manifest resolves whether it is stored in the Space World or
// supplied by the parent.
func (c *Controller) resolveFetchManifest(
	_ context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) ([]directive.Resolver, error) {
	if dir.GetManifestId() == "" {
		return nil, nil
	}

	resolvers := []directive.Resolver{directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		return c.resolveWorldFetchManifest(ctx, di, handler, dir)
	})}
	if c.manifestSource != nil {
		resolvers = append(resolvers, directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			return c.resolveSourceFetchManifest(ctx, handler, dir)
		}))
	}
	return resolvers, nil
}

// resolveWorldFetchManifest registers a resolver entry for processResolvers
// until ctx ends. The entry follows whether di has a value from any resolver.
func (c *Controller) resolveWorldFetchManifest(
	ctx context.Context,
	di directive.Instance,
	handler directive.ResolverHandler,
	dir manifest.FetchManifest,
) error {
	entry := &resolverEntry{ctx: ctx, dir: dir, handler: handler}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.resolvers[entry] = struct{}{}
		broadcast()
	})
	c.resolverBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	releaseState := di.AddStateCallback(func(_ bool, _ []error, vals []directive.AttachedValue) {
		resolved := len(vals) != 0
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if entry.resolved != resolved {
				entry.resolved = resolved
				broadcast()
			}
		})
	})
	defer func() {
		releaseState()
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			delete(c.resolvers, entry)
			broadcast()
		})
		c.resolverBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}()

	<-ctx.Done()
	return ctx.Err()
}

// runWorldWatchLoop runs the world watch loop that reconciles LoadPlugin
// directives from SpaceSettings and processes FetchManifest resolvers.
// Always runs while the controller is alive (not gated on resolver presence).
func (c *Controller) runWorldWatchLoop(ctx context.Context, engineID string) error {
	le := c.GetLogger()

	refs := make(map[string]pluginReference)
	defer func() {
		c.processes.ClearContext()
		for _, ref := range refs {
			ref.release()
		}
		c.loadedPlugins.Reset()
	}()
	c.processes.SetContext(ctx, true)

	watchLoop := world_control.NewWatchLoop(le, "", world_control.NewWaitForStateHandler(func(
		ctx context.Context,
		ws world.WorldState,
		_ world.ObjectState,
		_ *block.Cursor,
		_ uint64,
	) (bool, error) {
		c.reconcilePlugins(ctx, ws, refs)
		c.processResolvers(ctx, ws)
		c.reconcileProcesses(ctx, ws)
		return true, nil
	}))
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.watchLoop = watchLoop
		broadcast()
	})
	defer c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.watchLoop == watchLoop {
			c.watchLoop = nil
			broadcast()
		}
	})

	// Wake the watch loop when the resolver set changes.
	go func() {
		for {
			var ch <-chan struct{}
			c.resolverBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				ch = getWaitCh()
			})
			select {
			case <-ctx.Done():
				return
			case <-ch:
				le.Debug("resolver set changed, waking watch loop")
				watchLoop.Wake()
			}
		}
	}()

	return world_control.ExecuteBusWatchLoop(ctx, c.GetBus(), engineID, false, watchLoop)
}

// reconcilePlugins reads SpaceSettings from the world and reconciles
// LoadPlugin directives based on the current plugin_ids.
func (c *Controller) reconcilePlugins(ctx context.Context, ws world.WorldState, refs map[string]pluginReference) {
	le := c.GetLogger()
	conf := c.GetConfig()

	settings, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to lookup SpaceSettings")
		return
	}

	var ids []string
	if settings != nil {
		ids = pluginids.FilterValid(le, settings.GetPluginIds())
	}

	// Update the stored pluginIDs for FetchManifest filtering.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !slices.Equal(c.pluginIDs, ids) {
			c.pluginIDs = ids
			broadcast()
		}
	})

	// Build set of desired plugin IDs.
	desired := make(map[string]struct{}, len(ids))
	for _, pid := range ids {
		desired[pid] = struct{}{}
	}
	c.loadedPlugins.Reconcile(ids)

	// Release directives for plugins removed from SpaceSettings.
	for pid, ref := range refs {
		if _, ok := desired[pid]; !ok {
			ref.release()
			delete(refs, pid)
		}
	}

	// Add a replacement demand before releasing the previous one. The scheduler
	// retains the admitted worker while preparing this exact installation target.
	for _, pid := range ids {
		keys := settings.GetPluginInstallations()[pid].GetManifestKeys()
		previous, existed := refs[pid]
		if existed && slices.Equal(previous.manifestKeys, keys) {
			continue
		}
		demand := bldr_plugin.NewLoadPluginInstanced(pid, conf.GetSpaceId())
		if len(keys) != 0 {
			var selected []*manifest.ManifestRef
			for _, key := range keys {
				ref, err := space_world.LookupSpacePluginManifest(ctx, ws, pid, key)
				if err != nil {
					warnOnErrorUnlessCanceled(ctx, le, err, "failed to resolve installed plugin artifact")
					continue
				}
				selected = append(selected, ref)
			}
			if len(selected) == 0 {
				continue
			}
			demand = bldr_plugin.NewLoadPluginWithManifests(pid, conf.GetSpaceId(), selected...)
		}

		loadTarget := c.loadTarget
		if loadTarget == nil {
			loadTarget = c.GetBus()
		}
		di, ref, err := loadTarget.AddDirective(
			demand,
			nil,
		)
		if err != nil {
			warnOnErrorUnlessCanceled(ctx, le, err, "failed to add LoadPlugin directive")
			c.loadedPlugins.SetPluginState(pid, false, true)
			continue
		}
		pluginID := pid
		releasePrevious := sync.OnceFunc(func() {
			if existed {
				previous.release()
			}
		})
		if existed {
			previous.releaseState()
		}
		releaseState := di.AddStateCallback(func(
			isIdle bool,
			_ []error,
			vals []directive.AttachedValue,
		) {
			running := false
			if isIdle {
				for _, val := range vals {
					if _, ok := val.GetValue().(bldr_plugin.RunningPlugin); ok {
						running = true
						break
					}
				}
			}
			c.loadedPlugins.SetPluginState(pluginID, running, isIdle)
			if running {
				releasePrevious()
			}
		})
		refs[pid] = pluginReference{
			ref:             ref,
			releaseState:    releaseState,
			manifestKeys:    slices.Clone(keys),
			releasePrevious: releasePrevious,
		}
	}
}

// reconcileProcesses reads process bindings from the platform-account
// ObjectStore and starts/stops processes based on their binding state. A
// binding whose World object no longer exists is deleted, and its process
// stops.
func (c *Controller) reconcileProcesses(ctx context.Context, ws world.WorldState) {
	le := c.GetLogger()
	conf := c.GetConfig()

	volumeID := conf.GetVolumeId()
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	objectStoreID := conf.GetObjectStoreId()
	if objectStoreID == "" {
		objectStoreID = "platform-account"
	}

	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		c.GetBus(),
		true,
		objectStoreID,
		volumeID,
		nil,
	)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to get object store for process bindings")
		c.reconcileProcessConfigs(le, nil)
		return
	}
	if handle == nil || ref == nil {
		if ctx.Err() == nil {
			le.Warn("process binding object store unavailable")
		}
		c.reconcileProcessConfigs(le, nil)
		return
	}
	defer ref.Release()

	spaceID := conf.GetSpaceId()
	store := handle.GetObjectStore()
	bindings, err := process_binding.ListProcessBindings(ctx, store, spaceID)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to list process bindings")
		return
	}
	bindings, err = c.deleteOrphanedBindings(ctx, le, ws, store, spaceID, bindings)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to delete process bindings of deleted objects")
		return
	}

	// Build set of desired enabled bindings keyed by objectKey.
	desired := make(map[string]processConfig, len(bindings))
	for _, b := range bindings {
		if b.GetState() == s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED {
			desired[b.GetObjectKey()] = processConfig{
				typeID: b.GetTypeId(),
				ws:     ws,
			}
		}
	}
	c.reconcileProcessConfigs(le, desired)
}

// deleteOrphanedBindings deletes the bindings whose World object no longer
// exists and returns the others. An approval covers one object, so a later
// object at the same key needs a new decision.
func (c *Controller) deleteOrphanedBindings(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	store kvtx.Store,
	spaceID string,
	bindings []*s4wave_process.ProcessBinding,
) ([]*s4wave_process.ProcessBinding, error) {
	keys := make([]string, len(bindings))
	for i, binding := range bindings {
		keys[i] = binding.GetObjectKey()
	}
	refs, err := world.GetObjectRootRefsBatch(ctx, ws, keys)
	if err != nil {
		return nil, err
	}

	kept := make([]*s4wave_process.ProcessBinding, 0, len(bindings))
	var deleted bool
	for i, binding := range bindings {
		if refs[i].Exists {
			kept = append(kept, binding)
			continue
		}
		if err := process_binding.DeleteProcessBinding(ctx, store, spaceID, binding); err != nil {
			return nil, err
		}
		le.WithField("object-key", binding.GetObjectKey()).Info("deleted process binding of deleted object")
		deleted = true
	}
	if deleted && c.bindingsChanged != nil {
		c.bindingsChanged()
	}
	return kept, nil
}

// reconcileProcessConfigs starts and stops process routines to match desired.
func (c *Controller) reconcileProcessConfigs(le *logrus.Entry, desired map[string]processConfig) {
	active := c.processes.GetKeysWithData()
	le.WithField("enabled", len(desired)).WithField("active", len(active)).Debug("reconciling space processes")

	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.processConfigs = desired
	})

	desiredKeys := make([]string, 0, len(desired))
	for key := range desired {
		desiredKeys = append(desiredKeys, key)
	}
	added, removed := c.processes.SyncKeys(desiredKeys, false)

	for _, key := range removed {
		le.WithField("object-key", key).Debug("stopping process (removed or disabled)")
	}
	for _, key := range added {
		cfg := desired[key]
		le.WithField("object-key", key).WithField("type-id", cfg.typeID).Debug("starting process")
	}
	for _, entry := range c.processes.GetKeysWithData() {
		cfg, ok := desired[entry.Key]
		if !ok {
			continue
		}
		if entry.Data.typeID == cfg.typeID {
			continue
		}
		typeID := cfg.typeID
		le.WithField("object-key", entry.Key).
			WithField("type-id", typeID).
			Debug("restarting process after binding change")
		c.processes.ResetRoutine(entry.Key, func(_ string, data processConfig) bool {
			return data.typeID != typeID
		})
	}
}

// buildProcessRoutine builds the keyed process routine for one object key.
func (c *Controller) buildProcessRoutine(objectKey string) (keyed.Routine, processConfig) {
	cfg := c.getProcessConfig(objectKey)
	return func(ctx context.Context) error {
		if cfg.typeID == "" || cfg.ws == nil {
			return nil
		}
		return c.runProcess(ctx, cfg.ws, objectKey, cfg.typeID)
	}, cfg
}

// getProcessConfig returns the current enabled process configuration.
func (c *Controller) getProcessConfig(objectKey string) processConfig {
	var cfg processConfig
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cfg = c.processConfigs[objectKey]
	})
	return cfg
}

// runProcess resolves the ObjectType, creates an SRPC client, and runs
// the Execute streaming RPC until the stream closes or an error occurs.
func (c *Controller) runProcess(ctx context.Context, ws world.WorldState, objectKey, typeID string) error {
	b := c.GetBus()

	// Inject session peer ID and engine ID into context for factories.
	conf := c.GetConfig()
	if spid := conf.GetSessionPeerId(); spid != "" {
		pid, err := peer.IDB58Decode(spid)
		if err == nil {
			ctx = objecttype.WithSessionPeerID(ctx, pid)
		}
	}
	var engine world.Engine
	if eid := conf.GetEngineId(); eid != "" {
		ctx = objecttype.WithEngineID(ctx, eid)
		busEngine := world.NewBusEngine(ctx, b, eid)
		defer busEngine.ClearContext()
		engine = busEngine
	}

	ot, otRef, err := objecttype.ExLookupObjectType(ctx, b, typeID)
	if err != nil {
		return errors.Wrap(err, "lookup object type")
	}
	if ot == nil {
		return errors.New("object type not found: " + typeID)
	}
	defer otRef.Release()

	le := c.GetLogger()
	factory := ot.GetFactory()
	invoker, cleanup, err := factory(ctx, le, b, engine, ws, objectKey)
	if err != nil {
		return errors.Wrap(err, "object type factory")
	}
	if invoker == nil {
		// Type does not support process execution.
		if cleanup != nil {
			cleanup()
		}
		return nil
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create an in-process SRPC client from the invoker.
	srv := srpc.NewServer(invoker)
	client := srpc.NewClient(srpc.NewServerPipe(srv))
	execClient := s4wave_process.NewSRPCPersistentExecutionServiceClient(client)

	strm, err := execClient.Execute(ctx, &s4wave_process.ExecuteRequest{})
	if err != nil {
		return errors.Wrap(err, "execute RPC")
	}

	// Read status messages until the stream closes.
	for {
		status, err := strm.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.Wrap(err, "process stream")
		}
		le := c.GetLogger().
			WithField("object-key", objectKey).
			WithField("state", status.GetState().String())
		if errMsg := status.GetError(); errMsg != "" {
			le = le.WithField("error", errMsg)
		}
		le.Debug("process status update")
	}
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
