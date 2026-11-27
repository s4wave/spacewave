package plugin_host_scheduler

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_export "github.com/s4wave/spacewave/bldr/plugin/host/export"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	web_view_server "github.com/s4wave/spacewave/bldr/web/view/server"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/plugin/host/scheduler"

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// Controller implements the plugin host scheduler controller.
//
// Manages available plugin hosts and running plugins.
// Manages downloading the most appropriate manifest for the available plugin hosts.
// Manages executing plugins via the plugin hosts.
//
// Only one of these controllers should be running.
type Controller struct {
	// le is the root logger.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus
	// hostBus supplies live hosts. The bus is bound before Execute.
	hostBus bus.Bus
	// conf is the config.
	conf *Config
	// hostClient is a srpc client for the host mux for serving rpcs originating from the plugins.
	hostClient srpc.Client
	// objKey is the PluginHost object key (from the config).
	objKey string
	// peerID is the parsed peer id for sending world ops.
	peerID peer.ID
	// peerIDStr is the parsed peer id string.
	peerIDStr string

	// these values are set during Execute()

	// worldStateCtr contains the world state handle.
	worldStateCtr *ccontainer.CContainer[world.WorldState]
	// hostVolumeCtr is a container with the host volume.
	hostVolumeCtr *ccontainer.CContainer[*hostVol]
	// pluginHostsCtr is a container with the set of available plugin hosts.
	pluginHostsCtr *ccontainer.CContainer[*pluginHostSet]
	// manifestCopyGateCtr delays startup copies until runtime readiness.
	manifestCopyGateCtr *ccontainer.CContainer[ManifestCopyGate]
	// manifestStoreMtx guards manifest store initialization state.
	manifestStoreMtx sync.Mutex
	// manifestStoreReady caches successful manifest store initialization.
	manifestStoreReady bool
	// manifestStoreInit closes when the current initialization attempt completes.
	manifestStoreInit chan struct{}

	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.KeyedRefCount[pluginReference, *pluginInstance]
	// pluginStatusCtr publishes live plugin instance states.
	pluginStatusCtr *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
	// pluginStatusMtx guards the plugin status maps below.
	pluginStatusMtx sync.Mutex
	// pluginStatus stores live plugin instance states by pluginInstanceKey.
	pluginStatus map[string]*bldr_plugin.PluginStatus
	// pluginManifestRecoveryStatus stores retained Manifest selection and
	// eligibility facts by pluginInstanceKey.
	pluginManifestRecoveryStatus map[string]*bldr_plugin.PluginManifestRecoveryStatus
	// pluginExecutionStatus stores the running manifest and swap state of
	// each logical plugin instance by pluginInstanceKey.
	pluginExecutionStatus map[string]*bldr_plugin.PluginExecutionStatus
}

// hostVol contains a snapshot of the host volume.
type hostVol struct {
	vol  volume.Volume
	info *volume.VolumeInfo
}

// pluginHostSet is the set of plugin hosts snapshot.
type pluginHostSet struct {
	// pluginHosts retains the actual host identities reported by the bus.
	pluginHosts []bldr_plugin_host.PluginHost
	// err reports lookup failures without withdrawing unrelated live hosts.
	err error
	// settled reports that the lookup was idle when this set was taken: every
	// resolver running at that moment had reported its hosts. A host that
	// registers later, such as a web runtime opened afterwards, publishes a
	// new set.
	settled bool
}

// toPlatformIDsMap converts the host set to a map of platform ids to plugin hosts.
func (s *pluginHostSet) toPlatformIDsMap() map[string]bldr_plugin_host.PluginHost {
	// An empty host set has no platform map.
	if s == nil || len(s.pluginHosts) == 0 {
		return nil
	}

	// Map each platform id to the first host that serves it.
	hostMap := make(map[string]bldr_plugin_host.PluginHost)
	for _, h := range s.pluginHosts {
		if _, ok := hostMap[h.GetPlatformId()]; !ok {
			hostMap[h.GetPlatformId()] = h
		}
	}
	return hostMap
}

// toPluginPlatformIDsMap converts the host set to a policy-filtered map of
// platform ids to plugin hosts for one plugin.
func (s *pluginHostSet) toPluginPlatformIDsMap(conf *Config, pluginID string) map[string]bldr_plugin_host.PluginHost {
	// Build the platform map and drop disallowed platforms.
	hostMap := s.toPlatformIDsMap()
	if len(hostMap) == 0 {
		return nil
	}
	for platformID := range hostMap {
		if !conf.pluginPlatformAllowed(pluginID, platformID) {
			delete(hostMap, platformID)
		}
	}
	return hostMap
}

// pluginHostSetEqual reports whether two plugin host sets are equal.
func pluginHostSetEqual(a, b *pluginHostSet) bool {
	// Nil sets are equal only to each other.
	if a == nil || b == nil {
		return a == b
	}

	// Different errors or lengths are unequal.
	if a.err != b.err || a.settled != b.settled || len(a.pluginHosts) != len(b.pluginHosts) {
		return false
	}

	// Compare the host multisets.
	counts := make(map[bldr_plugin_host.PluginHost]int, len(a.pluginHosts))
	for _, host := range a.pluginHosts {
		counts[host]++
	}
	for _, host := range b.pluginHosts {
		counts[host]--
		if counts[host] < 0 {
			return false
		}
	}
	return true
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: conf.Validate enforces a valid peer id upstream, so the discard
	// here only tolerates configs that have not passed Validate yet.
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:                  le,
		bus:                 bus,
		hostBus:             bus,
		conf:                conf,
		objKey:              conf.GetObjectKey(),
		peerID:              peerID,
		peerIDStr:           peerID.String(),
		worldStateCtr:       ccontainer.NewCContainer[world.WorldState](nil),
		hostVolumeCtr:       ccontainer.NewCContainer[*hostVol](nil),
		pluginHostsCtr:      ccontainer.NewCContainerWithEqual(nil, pluginHostSetEqual),
		manifestCopyGateCtr: ccontainer.NewCContainer[ManifestCopyGate](nil),
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&bldr_plugin.PluginStatusSnapshot{},
			(*bldr_plugin.PluginStatusSnapshot).EqualVT,
		),
		pluginStatus:                 make(map[string]*bldr_plugin.PluginStatus),
		pluginManifestRecoveryStatus: make(map[string]*bldr_plugin.PluginManifestRecoveryStatus),
	}
	c.pluginInstances = keyed.NewKeyedRefCountWithLogger(c.newPluginInstance, le.WithField("tracker", "running-plugin"))
	c.hostClient = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(bus, "plugin-host", true))))
	return c
}

// SetManifestCopyGate sets the runtime readiness gate for startup manifest copies.
// A nil gate preserves immediate copy behavior.
func (c *Controller) SetManifestCopyGate(gate ManifestCopyGate) {
	c.manifestCopyGateCtr.SetValue(gate)
}

// getManifestCopyGate returns the gate serializing manifest copies.
func (c *Controller) getManifestCopyGate() ManifestCopyGate {
	if c == nil || c.manifestCopyGateCtr == nil {
		return nil
	}
	return c.manifestCopyGateCtr.GetValue()
}

// ensureManifestStore creates the scheduler manifest store before a plugin reads it.
func (c *Controller) ensureManifestStore(ctx context.Context) error {
	for {
		c.manifestStoreMtx.Lock()
		if c.manifestStoreReady {
			c.manifestStoreMtx.Unlock()
			return nil
		}
		if wait := c.manifestStoreInit; wait != nil {
			c.manifestStoreMtx.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return context.Canceled
			}
		}
		c.manifestStoreInit = make(chan struct{})
		c.manifestStoreMtx.Unlock()

		// A World this peer only reads keeps the store its writers made, and
		// a missing store reads as empty.
		engine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
		_, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, engine, c.objKey)
		if errors.Is(err, tx.ErrNotWrite) {
			err = nil
		}

		c.manifestStoreMtx.Lock()
		if err == nil {
			c.manifestStoreReady = true
		}
		close(c.manifestStoreInit)
		c.manifestStoreInit = nil
		c.manifestStoreMtx.Unlock()
		return err
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"plugin host scheduler",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	// Log the scheduler start.
	c.le.Info("starting plugin host scheduler")

	// Derive the scheduler's context from the lifecycle context.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// lookup the host volume
	vol, _, volRef, err := volume.ExLookupVolume(ctx, c.bus, c.conf.GetVolumeId(), "", false)
	if err != nil {
		return err
	}
	defer volRef.Release()

	// Describe the volume for the plugin host.
	volInfo, err := volume.NewVolumeInfo(ctx, controller.NewInfo(
		"hydra/volume/plugin-host",
		volume_rpc_server.Version,
		"proxy to plugin host volume",
	), vol)
	if err != nil {
		return err
	}

	// Publish the host volume to the scheduler's container.
	c.hostVolumeCtr.SetValue(&hostVol{
		vol:  vol,
		info: volInfo,
	})
	defer c.hostVolumeCtr.SetValue(nil)

	// construct the world engine handle
	var busEngine world.Engine = world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	if c.conf.GetVerbose() {
		busEngine = world_vlogger.NewEngine(c.le, busEngine)
	}
	ws := world.NewEngineWorldState(busEngine, true)

	// Publish the world state to the scheduler's container.
	c.worldStateCtr.SetValue(ws)
	defer c.worldStateCtr.SetValue(nil)

	// startup manifest watchers & plugin instances
	c.pluginInstances.SetContext(ctx, true)
	defer c.pluginInstances.ClearContext()

	// Retain the materializer for this execution without blocking the host
	// discovery below that resolves its startup.
	if matID := c.conf.GetMaterializerPluginId(); matID != "" && !c.conf.GetDisableCopyManifest() {
		_, matRef, err := c.bus.AddDirective(
			ctx,
			bldr_plugin.NewLoadPluginInstanced(matID, c.conf.GetInstanceKey()),
			nil,
		)
		if err != nil {
			return err
		}
		defer matRef.Release()
	}

	// Watch the plugin host lookup. The state callback delivers the values,
	// resolver errors and idle flag as one snapshot in the order the lookup
	// changed, so a set marked settled holds every host reported before idle.
	// The first set is published once the lookup has gone idle.
	lookup, lookupRef, err := c.hostBus.AddDirective(ctx, bldr_plugin_host.NewLookupPluginHost(nil), nil)
	if err != nil {
		return err
	}
	defer lookupRef.Release()

	// Publish each snapshot from the first idle one on. seenIdle is touched
	// only by the state callback, which the instance calls one at a time.
	var seenIdle bool
	relState := lookup.AddStateCallback(func(idle bool, resErr []error, vals []directive.AttachedValue) {
		// Hold back the partial sets the lookup reports before it first idles.
		seenIdle = seenIdle || idle
		if !seenIdle {
			return
		}

		// Publish the hosts among the values with the snapshot's idle flag.
		hosts := make([]bldr_plugin_host.PluginHost, 0, len(vals))
		for _, v := range vals {
			if host, ok := v.GetValue().(bldr_plugin_host.PluginHost); ok {
				hosts = append(hosts, host)
			}
		}
		c.publishHosts(resErr, hosts, idle)
	})
	defer relState()

	// Report a disposed lookup as an unsettled error beside the last hosts.
	relDispose := lookup.AddDisposeCallback(func() {
		var hosts []bldr_plugin_host.PluginHost
		if prev := c.pluginHostsCtr.GetValue(); prev != nil {
			hosts = prev.pluginHosts
		}
		c.publishHosts([]error{directive.ErrDirectiveDisposed}, hosts, false)
	})
	defer relDispose()

	// Keep the watch until the scheduler stops.
	<-ctx.Done()
	return context.Canceled
}

// resolveLoadPlugin resolves a LoadPlugin directive.
//
// A directive without an instance key loads the configured instance. Other
// keys start further instances on this scheduler. The bus the scheduler runs
// on bounds which callers can reach the instances.
func (c *Controller) resolveLoadPlugin(dir bldr_plugin.LoadPlugin) (directive.Resolver, error) {
	if !c.conf.SchedulesPlugin(dir.LoadPluginID()) {
		return nil, nil
	}
	bindingKey, err := c.loadBindingKey(dir)
	if err != nil {
		return nil, err
	}
	return bldr_plugin_host.NewLoadPluginResolver(c, dir, bindingKey), nil
}

// loadBindingKey returns the logical instance dir runs on this scheduler.
//
// Invariant: a host plugin (HostExportPluginIds) runs only as this
// scheduler's own instance, which holds the whole host bus and volume. A load
// of one under another key, such as a Space's, binds to that instance, so no
// key starts a second copy with host powers. Such a load may not select its
// build: one referencing manifests or a manifest root is refused, so a Space's
// admitted manifest never replaces the host's build.
func (c *Controller) loadBindingKey(dir bldr_plugin.LoadPlugin) (string, error) {
	// Any plugin outside the host set, or any load at the own key, keeps its key.
	pluginID := dir.LoadPluginID()
	bindingKey, ownKey := c.bindingKey(dir.LoadPluginInstanceKey()), c.bindingKey("")
	if bindingKey == ownKey || !slices.Contains(c.conf.GetHostExportPluginIds(), pluginID) {
		return bindingKey, nil
	}

	// A host plugin under a foreign key joins the own instance, unpinned.
	if len(dir.LoadPluginManifests()) != 0 || dir.LoadPluginManifestRoot() != "" {
		return "", errors.Errorf("host plugin %s cannot select a build at instance %q", pluginID, bindingKey)
	}
	return ownKey, nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context tasked is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case bldr_plugin.LoadPlugin:
		return directive.R(c.resolveLoadPlugin(d))
	case bifrost_rpc.LookupRpcClient:
		return directive.R(c.resolveLookupRpcClient(ctx, d))
	case bifrost_rpc.LookupRpcService:
		return directive.R(c.resolveLookupRpcService(ctx, d))
	case bldr_plugin.LookupPluginScheduler:
		return directive.R(directive.NewValueResolver([]bldr_plugin.LookupPluginSchedulerValue{c}), nil)
	}
	return nil, nil
}

// resolveLookupRpcService resolves a lookup of a plugin's or the host's
// services with that plugin's root client.
//
// Invariant: no call that originated in a plugin outside the host's set
// reaches another plugin's root client. On the host bus the identity of a call
// is the server ID the host itself assigned, never bytes the plugin sent. A
// plugin outside the set reaches other plugins only through its own host
// invoker, which delivers through PluginRpc under its binding. A lookup the
// rule refuses fails at once instead of waiting for another resolver.
func (c *Controller) resolveLookupRpcService(ctx context.Context, d bifrost_rpc.LookupRpcService) (directive.Resolver, error) {
	// Leave lookups of services that no plugin root serves to other resolvers.
	res, err := bldr_plugin.ResolveLookupRpcService(ctx, d, c)
	if res == nil || err != nil {
		return res, err
	}

	// Refuse a server ID the host did not give a host plugin.
	serverID := d.LookupRpcServerID()
	if !bldr_plugin.MayReachPluginRoot(serverID, c.conf.GetHostExportPluginIds()) {
		return bldr_plugin.NewRelayRefusedResolver(serverID), nil
	}
	return res, nil
}

// resolveLookupRpcClient resolves a lookup of a plugin's or the host's client,
// as resolveLookupRpcService does a service, keyed on the caller's client ID.
func (c *Controller) resolveLookupRpcClient(ctx context.Context, d bifrost_rpc.LookupRpcClient) (directive.Resolver, error) {
	// Leave lookups of clients that no plugin root serves to other resolvers.
	res, err := bldr_plugin.ResolveLookupRpcClient(ctx, d, c)
	if res == nil || err != nil {
		return res, err
	}

	// Refuse a client ID the host did not give a host plugin.
	clientID := d.LookupRpcClientID()
	if !bldr_plugin.MayReachPluginRoot(clientID, c.conf.GetHostExportPluginIds()) {
		return bldr_plugin.NewRelayRefusedResolver(clientID), nil
	}
	return res, nil
}

// GetHostPluginIds returns the IDs of the plugins this scheduler starts as
// host plugins.
func (c *Controller) GetHostPluginIds() []string {
	return c.conf.GetHostExportPluginIds()
}

// pluginInstanceKey builds the deduplication key for pluginInstances.
// When instanceKey is empty, uses pluginID alone (shared instance).
// When non-empty, uses pluginID/instanceKey (instanced).
func pluginInstanceKey(pluginID, instanceKey string) string {
	if instanceKey == "" {
		return pluginID
	}
	return pluginID + "/" + instanceKey
}

// AddPluginReference adds a reference to the plugin, returning the RunningPlugin
// handle and a release function.
// instanceKey may be empty for shared (non-instanced) plugins.
func (c *Controller) AddPluginReference(pluginID, instanceKey string) (bldr_plugin.RunningPluginRef, func()) {
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginReference{pluginID: pluginID, instanceKey: instanceKey})
	plg.start.SetValue(true)
	return plg, ref.Release
}

// RestartPlugin ends the running execution of the plugin's binding under
// instanceKey, so the scheduler starts the binding's selected manifest at once.
// Pinned executions are not restarted. Returns false when no reference holds
// the binding.
func (c *Controller) RestartPlugin(pluginID, instanceKey string) bool {
	plg, ok := c.pluginInstances.GetKey(pluginReference{pluginID: pluginID, instanceKey: instanceKey})
	if ok {
		plg.restart()
	}
	return ok
}

// AddPinnedPluginReference retains an exact manifest in a separate host worker.
func (c *Controller) AddPinnedPluginReference(pluginID, instanceKey, manifestRoot string) (bldr_plugin.RunningPluginRef, func()) {
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginReference{
		pluginID: pluginID, instanceKey: instanceKey, manifestRoot: manifestRoot,
	})
	plg.start.SetValue(true)
	return plg, ref.Release
}

// AddSelectedPluginReference retains a desired artifact without replacing the
// binding.
// The newest retained selection wins. Removing the selection restores the
// preceding demand.
// Each root runs only once AdmitPluginRoot admits it for peerID.
func (c *Controller) AddSelectedPluginReference(pluginID, instanceKey, peerID string, selected ...*bldr_manifest.ManifestRef) (bldr_plugin.RunningPluginRef, func()) {
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginReference{pluginID: pluginID, instanceKey: instanceKey})
	releaseSelection := plg.addManifestSelection(peerID, selected...)
	plg.start.SetValue(true)
	return plg, sync.OnceFunc(func() {
		releaseSelection()
		ref.Release()
	})
}

// AddAssetsPluginReference retains a plugin whose files the caller serves to a
// browser. While any such reference exists, the binding mounts the files of a
// browser manifest when no host can run the plugin.
func (c *Controller) AddAssetsPluginReference(pluginID, instanceKey, manifestRoot string) (bldr_plugin.RunningPluginRef, func()) {
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginReference{pluginID: pluginID, instanceKey: instanceKey, manifestRoot: manifestRoot})
	releaseAssets := plg.addAssetsDemand()
	plg.start.SetValue(true)
	return plg, sync.OnceFunc(func() {
		releaseAssets()
		ref.Release()
	})
}

// WaitPluginHostClient waits for an RPC client for the plugin host.
//
// Released is a function to call if the client becomes invalid.
// Returns nil, nil, err if any error.
// Returns nil, nil, nil to skip resolving the client.
// Otherwise returns client, releaseFunc, nil
func (c *Controller) WaitPluginHostClient(ctx context.Context, released func()) (srpc.Client, func(), error) {
	return c.hostClient, nil, nil
}

// WaitPluginClient waits for an RPC client for a plugin the scheduler loads.
// It returns nil, nil, nil for any other plugin, so the lookup stays with the
// rest of the bus. Released is called if the client becomes invalid.
func (c *Controller) WaitPluginClient(ctx context.Context, released func(), pluginID string) (srpc.Client, func(), error) {
	// Reject invalid IDs and leave other plugins to the rest of the bus.
	if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
		return nil, nil, err
	}
	if !c.conf.SchedulesPlugin(pluginID) {
		return nil, nil, nil
	}

	// Load the plugin and wait for its client.
	client, ref, err := bldr_plugin.ExPluginLoadInstancedWaitClient(
		ctx,
		c.bus,
		pluginID,
		c.conf.GetInstanceKey(),
		released,
	)
	if err != nil {
		return nil, nil, err
	}
	return client, ref.Release, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// buildPluginMux builds the rpc mux for plugins.
//
// ctx should remain active as long as Mux is in use.
// Returns the mux and a release function for cleaning up resources.
func (c *Controller) buildPluginMux(
	ctx context.Context,
	pluginID string,
	manifest *bldr_manifest.ManifestSnapshot,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	proxyHostVolInfo *volume.VolumeInfo,
	distFS,
	assetsFS *unixfs.FSHandle,
	hostRoot *plugin_host_root.Root,
	registrationDone plugin_host_resource.InitialCapabilityRegistrationDoneFunc,
	historical, prepared bool,
	bindingKey string,
) (srpc.Mux, func()) {
	// Build the plugin host service, which also decides what the plugin
	// reaches on the host bus beyond the services registered here.
	hostExports := c.conf.GetHostExportPluginIds()
	hostServer := bldr_plugin_host.NewPluginHostServer(
		ctx,
		c.bus,
		c.le,
		bldr_plugin_host.PluginHostServerConfig{
			PluginID:                pluginID,
			InstanceKey:             c.conf.GetInstanceKey(),
			RegistrationInstanceKey: bindingKey,
			ManifestSnapshot:        manifest,
			HostVolumeInfo:          proxyHostVolInfo,
			HostStorageID:           c.conf.GetHostStorageId(),
			Historical:              historical,
			Prepared:                prepared,
			NamesPeers:              slices.Contains(hostExports, pluginID),
			HostExportPluginIDs:     hostExports,
			Scheduler:               c,
		},
	)

	// Register the plugin host service and its host bus fallback.
	mux := srpc.NewMux(hostServer.HostBusInvoker(c.conf.GetAllowedHostServiceIds()))
	_ = bldr_plugin.SRPCRegisterPluginHost(mux, hostServer)

	// register access host volume via rpc service
	_ = volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, proxyHostVol, bldr_plugin.HostVolumeServiceIDPrefix)

	// register access web views via bus service
	_ = web_view.SRPCRegisterAccessWebViews(mux, web_view_server.NewAccessWebViewsViaBus(c.le, c.bus))

	// register plugin dist fs service
	_ = mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
		unixfs_rpc_server.NewFSCursorServiceWithHandle(distFS),
		bldr_plugin.PluginDistServiceID,
	))

	// register plugin assets fs service
	_ = mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
		unixfs_rpc_server.NewFSCursorServiceWithHandle(assetsFS),
		bldr_plugin.PluginAssetsServiceID,
	))

	// register resource server for plugin resource access
	pluginHostRoot := plugin_host_resource.NewPluginHostRoot(
		c.bus, pluginID, manifest.GetManifest().GetEntrypoint(),
		distFS, assetsFS, proxyHostVol,
		hostRoot,
		"plugin-state-atoms",
		bldr_plugin.PluginVolumeID,
		registrationDone,
	)
	resourceSrv := resource_server.NewResourceServer(pluginHostRoot.GetMux(), ControllerID)
	_ = resourceSrv.Register(mux)

	// export this scheduler's plugin hosts to trusted plugins
	if slices.Contains(c.conf.GetHostExportPluginIds(), pluginID) {
		_ = plugin_host_export.SRPCRegisterHostExport(mux, plugin_host_export.NewServer(c.le, c.bus, c))
	}

	return mux, pluginHostRoot.Release
}

// SetPluginHostBus binds host discovery before Execute. Hosts remain owned by
// their publishing bus. The scheduler owns their watch and selection lifetime.
func (c *Controller) SetPluginHostBus(hostBus bus.Bus) {
	c.hostBus = hostBus
}

// _ is a type assertion
var (
	_ controller.Controller                = (*Controller)(nil)
	_ bldr_plugin.LookupRpcClientHandler   = (*Controller)(nil)
	_ bldr_plugin_host.PluginHostScheduler = (*Controller)(nil)
	_ bldr_plugin.HostPluginScheduler      = (*Controller)(nil)
)
