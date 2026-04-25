package plugin_space

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	plugin_list "github.com/s4wave/spacewave/core/plugin/list"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// ControllerID is the controller ID.
const ControllerID = "plugin/space"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "loads approved plugins and resolves FetchManifest for a Space"

type processConfig struct {
	typeID string
	ws     world.WorldState
}

var processRetryBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 1000,
		MaxInterval:     30000,
		Multiplier:      2,
	},
}

// Controller loads approved plugins for a Space and resolves FetchManifest
// directives by watching the Space world with approval gating.
//
// Watches SpaceSettings in the Space world reactively. When plugin_ids change
// in SpaceSettings, reconciles LoadPlugin directives: adds directives for
// newly-approved plugins, releases directives for removed plugins.
//
// For FetchManifest: resolves FetchManifest directives for manifest IDs
// matching the current SpaceSettings plugin_ids. Checks approval before
// returning manifest values. Uses a shared world watch loop with broadcast
// to handle resolver set changes.
//
// Also reconciles process bindings: starts approved persistent processes
// and stops processes that are removed or unapproved.
type Controller struct {
	*bus.BusController[*Config]

	// bcast guards resolvers and pluginIDs.
	bcast broadcast.Broadcast
	// resolvers is the set of active FetchManifest resolvers.
	resolvers map[*resolverEntry]struct{}
	// pluginIDs is the current set of plugin IDs from SpaceSettings.
	// Updated each world watch cycle. Protected by bcast.
	pluginIDs []string
	// processConfigs tracks the current approved process configuration by object key.
	processConfigs map[string]processConfig
	// processes tracks active process routines by object key.
	processes *keyed.Keyed[string, processConfig]
	// watchLoop is the active world watch loop while Execute is running.
	watchLoop *world_control.WatchLoop
}

// NotifyChanged wakes the watch loop to reconcile approval-backed state.
func (c *Controller) NotifyChanged() {
	var watchLoop *world_control.WatchLoop
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		watchLoop = c.watchLoop
		broadcast()
	})
	if watchLoop != nil {
		watchLoop.Wake()
	}
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
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
				BusController:  base,
				resolvers:      make(map[*resolverEntry]struct{}),
				processConfigs: make(map[string]processConfig),
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

	return c.runWorldWatchLoop(ctx, engineID)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case manifest.FetchManifest:
		return c.resolveFetchManifest(ctx, di, dir)
	case plugin_list.ListAvailablePlugins:
		return c.resolveListAvailablePlugins(ctx, di, dir)
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

// resolveFetchManifest handles a FetchManifest directive.
// Returns a resolver that persistently watches the plugin list.
// processResolvers handles actual resolution from world state.
func (c *Controller) resolveFetchManifest(
	_ context.Context,
	_ directive.Instance,
	dir manifest.FetchManifest,
) ([]directive.Resolver, error) {
	mid := dir.GetManifestId()
	if mid == "" {
		return nil, nil
	}

	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		entry := &resolverEntry{ctx: ctx, dir: dir, handler: handler}
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			c.resolvers[entry] = struct{}{}
			broadcast()
		})
		defer func() {
			c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				delete(c.resolvers, entry)
				broadcast()
			})
		}()

		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

// runWorldWatchLoop runs the world watch loop that reconciles LoadPlugin
// directives from SpaceSettings and processes FetchManifest resolvers.
// Always runs while the controller is alive (not gated on resolver presence).
func (c *Controller) runWorldWatchLoop(ctx context.Context, engineID string) error {
	le := c.GetLogger()

	refs := make(map[string]directive.Reference)
	defer func() {
		c.processes.ClearContext()
		for _, ref := range refs {
			ref.Release()
		}
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
			c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
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
// LoadPlugin directives based on the current plugin_ids and approval state.
func (c *Controller) reconcilePlugins(ctx context.Context, ws world.WorldState, refs map[string]directive.Reference) {
	le := c.GetLogger()

	settings, _, err := space_world.LookupSpaceSettings(ctx, ws)
	if err != nil {
		le.WithError(err).Warn("failed to lookup SpaceSettings")
		return
	}

	var ids []string
	if settings != nil {
		ids = settings.GetPluginIds()
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

	// Release directives for plugins removed from SpaceSettings.
	for pid, ref := range refs {
		if _, ok := desired[pid]; !ok {
			ref.Release()
			delete(refs, pid)
		}
	}

	// Add directives for newly-approved plugins.
	for _, pid := range ids {
		if _, ok := refs[pid]; ok {
			continue
		}

		approved, err := c.checkApproval(ctx, pid)
		if err != nil {
			le.WithError(err).Warn("failed to check plugin approval")
			continue
		}
		if !approved {
			continue
		}

		_, ref, err := c.GetBus().AddDirective(
			bldr_plugin.NewLoadPlugin(pid),
			nil,
		)
		if err != nil {
			le.WithError(err).Warn("failed to add LoadPlugin directive")
			continue
		}
		refs[pid] = ref
	}

	// Release directives for plugins that lost approval.
	for pid, ref := range refs {
		if _, ok := desired[pid]; !ok {
			continue
		}
		approved, err := c.checkApproval(ctx, pid)
		if err != nil {
			continue
		}
		if !approved {
			ref.Release()
			delete(refs, pid)
		}
	}
}

// checkApproval checks if a manifest ID is approved for the configured space.
func (c *Controller) checkApproval(ctx context.Context, mid string) (bool, error) {
	conf := c.GetConfig()
	return plugin_approval.CheckApproval(
		ctx,
		c.GetBus(),
		conf.GetVolumeId(),
		conf.GetObjectStoreId(),
		conf.GetSpaceId(),
		mid,
	)
}

// reconcileProcesses reads process bindings from the platform-account
// ObjectStore and starts/stops processes based on their approval state.
func (c *Controller) reconcileProcesses(ctx context.Context, ws world.WorldState) {
	le := c.GetLogger()
	conf := c.GetConfig()

	volumeID := conf.GetVolumeId()
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	objectStoreID := conf.GetObjectStoreId()
	if objectStoreID == "" {
		objectStoreID = plugin_approval.DefaultObjectStoreID
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
		le.WithError(err).Warn("failed to get object store for process bindings")
		return
	}
	defer ref.Release()

	spaceID := conf.GetSpaceId()
	bindings, err := process_binding.ListProcessBindings(ctx, handle.GetObjectStore(), spaceID)
	if err != nil {
		le.WithError(err).Warn("failed to list process bindings")
		return
	}

	// Build set of desired approved bindings keyed by objectKey.
	desired := make(map[string]processConfig, len(bindings))
	for _, b := range bindings {
		if b.GetState() == s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED {
			desired[b.GetObjectKey()] = processConfig{
				typeID: b.GetTypeId(),
				ws:     ws,
			}
		}
	}

	active := c.processes.GetKeysWithData()
	le.WithField("approved", len(desired)).WithField("active", len(active)).Debug("reconcileProcesses")

	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		c.processConfigs = desired
	})

	desiredKeys := make([]string, 0, len(desired))
	for key := range desired {
		desiredKeys = append(desiredKeys, key)
	}
	added, removed := c.processes.SyncKeys(desiredKeys, false)

	for _, key := range removed {
		le.WithField("object-key", key).Debug("stopping process (removed or unapproved)")
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

// getProcessConfig returns the current approved process configuration.
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
	if eid := conf.GetEngineId(); eid != "" {
		ctx = objecttype.WithEngineID(ctx, eid)
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
	invoker, cleanup, err := factory(ctx, le, b, nil, ws, objectKey)
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
		c.GetLogger().
			WithField("object-key", objectKey).
			WithField("state", status.GetState().String()).
			Debug("process status update")
	}
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
