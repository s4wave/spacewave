package plugin_host_scheduler

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	web_view_server "github.com/s4wave/spacewave/bldr/web/view/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/plugin/host/scheduler"

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the plugin host scheduler controller.
//
// Manages available plugin hosts and running plugins.
// Manages downloading the most appropriate manifest for the available plugin hosts.
// Manages executing plugins via the plugin hosts.
//
// Only one of these controllers should be running.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// hostClient is a srpc client for the host mux for serving rpcs originating from the plugins.
	hostClient srpc.Client
	// objKey is the PluginHost object key (from the config)
	objKey string
	// peerID is the parsed peer id for sending world ops
	peerID peer.ID
	// peerIDStr is the parsed peer id string
	peerIDStr string

	// these values are set during Execute()

	// worldStateCtr contains the world state handle.
	worldStateCtr *ccontainer.CContainer[world.WorldState]
	// hostVolumeCtr is a container with the host volume.
	hostVolumeCtr *ccontainer.CContainer[*hostVol]
	// pluginHostsCtr is a container with the set of available plugin hosts.
	pluginHostsCtr *ccontainer.CContainer[*pluginHostSet]

	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.KeyedRefCount[string, *pluginInstance]
}

// hostVol contains a snapshot of the host volume.
type hostVol struct {
	vol  volume.Volume
	info *volume.VolumeInfo
}

// pluginHostSet is the set of plugin hosts snapshot.
type pluginHostSet struct {
	pluginHosts []bldr_plugin_host.PluginHost
}

// toPlatformIDs converts the host set to a list of platform ids.
func (s *pluginHostSet) toPlatformIDs() []string {
	if s == nil || len(s.pluginHosts) == 0 {
		return nil
	}
	ids := make([]string, len(s.pluginHosts))
	for i, h := range s.pluginHosts {
		ids[i] = h.GetPlatformId()
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	return ids
}

// toPlatformIDsMap converts the host set to a map of platform ids to plugin hosts.
func (s *pluginHostSet) toPlatformIDsMap() map[string]bldr_plugin_host.PluginHost {
	if s == nil || len(s.pluginHosts) == 0 {
		return nil
	}
	hostMap := make(map[string]bldr_plugin_host.PluginHost)
	for _, h := range s.pluginHosts {
		hostMap[h.GetPlatformId()] = h
	}
	return hostMap
}

func pluginHostSetEqual(a, b *pluginHostSet) bool {
	if a == nil || b == nil {
		return a == b
	}
	return slices.Equal(a.pluginHosts, b.pluginHosts)
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:             le,
		bus:            bus,
		conf:           conf,
		objKey:         conf.GetObjectKey(),
		peerID:         peerID,
		peerIDStr:      peerID.String(),
		worldStateCtr:  ccontainer.NewCContainer[world.WorldState](nil),
		hostVolumeCtr:  ccontainer.NewCContainer[*hostVol](nil),
		pluginHostsCtr: ccontainer.NewCContainerWithEqual(nil, pluginHostSetEqual),
	}
	c.pluginInstances = keyed.NewKeyedRefCountWithLogger(c.newPluginInstance, le.WithField("tracker", "running-plugin"))
	c.hostClient = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(bus, "plugin-host", true))))
	return c
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
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	c.le.Info("starting plugin host scheduler")
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// lookup the host volume
	vol, _, volRef, err := volume.ExLookupVolume(ctx, c.bus, c.conf.GetVolumeId(), "", false)
	if err != nil {
		return err
	}
	defer volRef.Release()

	volInfo, err := volume.NewVolumeInfo(ctx, controller.NewInfo(
		"hydra/volume/plugin-host",
		volume_rpc_server.Version,
		"proxy to plugin host volume",
	), vol)
	if err != nil {
		return err
	}

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

	c.worldStateCtr.SetValue(ws)
	defer c.worldStateCtr.SetValue(ws)

	// startup manifest watchers & plugin instances
	c.pluginInstances.SetContext(ctx, true)
	defer c.pluginInstances.ClearContext()

	// watch list of plugin hosts
	errCh := make(chan error, 1)
	_, hostsRel, err := bus.ExecCollectValuesWatch(
		ctx,
		c.bus,
		bldr_plugin_host.NewLookupPluginHost(nil),
		// true: wait for directive to be idle before emitting initial set of values.
		true,
		func(resErr []error, vals []bldr_plugin_host.PluginHost) error {
			if len(resErr) != 0 {
				c.le.WithField("resolver-errs", resErr).Warn("one or more plugin hosts are erroring")
			}

			// check and warn if there are any duplicate platform ids (not currently handled well)
			var ids []string
			for _, pluginHost := range vals {
				ids = append(ids, pluginHost.GetPlatformId())
			}
			slices.Sort(ids)
			originalLen := len(ids)
			ids = slices.Compact(ids)

			// update the host set
			c.le.WithField("plugin-hosts", ids).Infof("scheduling with %d plugin host(s)", len(ids))
			hostSet := &pluginHostSet{pluginHosts: vals}
			c.pluginHostsCtr.SetValue(hostSet)

			// Warn if we have multiple plugin hosts with the same platform ID
			if originalLen > len(ids) {
				c.le.WithField("plugin-hosts", ids).Warn("detected multiple plugin hosts with the same platform id")
			}

			return nil
		},
		func(err error) { errCh <- err },
	)
	if err != nil {
		return err
	}
	defer hostsRel()

	// wait for context cancel or terminal error to cleanup
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(dir bldr_plugin.LoadPlugin) (directive.Resolver, error) {
	return bldr_plugin_host.NewLoadPluginResolver(c, dir.LoadPluginID(), dir.LoadPluginInstanceKey()), nil
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
		return directive.R(bldr_plugin.ResolveLookupRpcClient(ctx, d, c))
	case bifrost_rpc.LookupRpcService:
		return directive.R(bldr_plugin.ResolveLookupRpcService(ctx, d, c))
	}
	return nil, nil
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
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginInstanceKey(pluginID, instanceKey))
	return plg, ref.Release
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

// WaitPluginClient waits for an RPC client for a plugin.
//
// if pluginID is invalid, returns an error.
//
// Released is a function to call if the client becomes invalid.
// Returns nil, nil, err if any error.
// Returns nil, nil, nil to skip resolving the client.
// Otherwise returns client, releaseFunc, nil
func (c *Controller) WaitPluginClient(ctx context.Context, released func(), pluginID string) (srpc.Client, func(), error) {
	if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
		return nil, nil, err
	}

	client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, c.bus, pluginID, released)
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
) (srpc.Mux, func()) {
	// fallback to a LookupRpcService on the bus
	mux := srpc.NewMux(bifrost_rpc.NewInvoker(c.bus, bldr_plugin.PluginServerID(pluginID, ""), true))

	// register access host volume via rpc service
	_ = volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, proxyHostVol, bldr_plugin.HostVolumeServiceIDPrefix)

	// register access web views via bus service
	_ = web_view.SRPCRegisterAccessWebViews(mux, web_view_server.NewAccessWebViewsViaBus(c.le, c.bus))

	// register plugin host service
	_ = bldr_plugin.SRPCRegisterPluginHost(mux, bldr_plugin_host.NewPluginHostServer(ctx, c.bus, c.le, pluginID, manifest, proxyHostVolInfo))

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
		c.le, c.bus, pluginID, manifest.GetManifest().GetEntrypoint(),
		distFS, assetsFS, proxyHostVol,
		"plugin-state-atoms",
		bldr_plugin.PluginVolumeID,
	)
	resourceSrv := resource_server.NewResourceServer(pluginHostRoot.GetMux())
	_ = resourceSrv.Register(mux)

	return mux, pluginHostRoot.Release
}

/*
// cleanupUnknownPlugins calls DeletePlugin for any plugins without a matching manifest.
func (c *Controller) cleanupUnknownPlugins(ctx context.Context, ws world.WorldState, filterPlatformID string, host bldr_plugin_host.PluginHost) error {
	// list ids from the plugin host
	loadedPlugins, err := host.ListPlugins(ctx)
	if err != nil {
		return err
	}

	// if there are no known plugin ids stop here
	if len(loadedPlugins) == 0 {
		return nil
	}

	// fetch all known plugin manifests
	pluginManifests, pluginManifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, filterPlatformID, c.objKey)
	if err != nil {
		return err
	}
	for _, err := range pluginManifestErrs {
		c.le.WithError(err).Warn("ignoring invalid plugin manifest")
	}

	// delete any unknowns
	var unknownPlugins []string
	for _, loadedPlugin := range loadedPlugins {
		if _, ok := pluginManifests[loadedPlugin]; !ok {
			unknownPlugins = append(unknownPlugins, loadedPlugin)
		}
	}
	if len(unknownPlugins) == 0 {
		return nil
	}

	c.le.Infof("clearing %d unknown / out of date plugins", len(unknownPlugins))
	for _, unknownPlugin := range unknownPlugins {
		if err := host.DeletePlugin(ctx, unknownPlugin); err != nil {
			if err == context.Canceled {
				return err
			}
			c.le.WithError(err).Warnf("unable to clear old plugin: %s", unknownPlugin)
		}
	}

	return nil
}
*/

// _ is a type assertion
var (
	_ controller.Controller                = ((*Controller)(nil))
	_ bldr_plugin.LookupRpcClientHandler   = ((*Controller)(nil))
	_ bldr_plugin_host.PluginHostScheduler = ((*Controller)(nil))
)
