package devtool_web_entrypoint_plugin_host

import (
	"context"
	"sync"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_server "github.com/aperturerobotics/bldr/web/view/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_server "github.com/aperturerobotics/hydra/unixfs/rpc/server"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/sirupsen/logrus"
)

// Controller implements the web devtool entrypoint PluginHost controller.
//
// This is a simplified version of the plugin/host/controller.
// This version accesses plugins from the devtool without the world management or fetching logic.
//
// The implementation could be unified with plugin/host/controller if the world
// logic was split out into a separate controller. However, it is simpler to
// just implement the special case of the devtool here for now.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// info contains the controller info
	info *controller.Info
	// sfs is used to decode manifests
	sfs *block_transform.StepFactorySet
	// host is the plugin host
	host bldr_plugin_host.PluginHost
	// hostClient is a loopback srpc client to the host.
	hostClient srpc.Client
	// hostVolumeCtr is a container with the host volume.
	hostVolumeCtr *ccontainer.CContainer[*hostVol]
	// hostPluginPlatformID is the plugin platform ID to use.
	hostPluginPlatformID *promise.PromiseContainer[string]
	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.KeyedRefCount[string, *pluginTracker]
	// pluginManifestTrackers manages watching manifests for running plugins.
	// key: pluginID
	// controlled by pluginInstances
	pluginManifestTrackers *keyed.Keyed[string, *pluginManifestTracker]
	// mtx guards below fields
	mtx sync.Mutex
	// pluginManifests contains the latest known manifest for the loaded plugins.
	// managed by pluginManifestTrackers
	pluginManifests map[string]*bldr_manifest.ManifestSnapshot
}

// hostVol contains a snapshot of the host volume.
type hostVol struct {
	vol  volume.Volume
	info *volume.VolumeInfo
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	info *controller.Info,
	host bldr_plugin_host.PluginHost,
) *Controller {
	sfs := transform_all.BuildFactorySet()
	c := &Controller{
		le:                   le,
		bus:                  bus,
		conf:                 conf,
		info:                 info,
		sfs:                  sfs,
		host:                 host,
		hostPluginPlatformID: promise.NewPromiseContainer[string](),
		pluginManifests:      make(map[string]*bldr_manifest.ManifestSnapshot),
	}
	c.pluginManifestTrackers = keyed.NewKeyedWithLogger(c.newPluginManifestTracker, le.WithField("tracker", "manifest-watcher"))
	c.pluginInstances = keyed.NewKeyedRefCountWithLogger(c.newRunningPlugin, le.WithField("tracker", "plugin-instances"))
	c.hostClient = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(bus, "plugin-host", true))))
	c.hostVolumeCtr = ccontainer.NewCContainer[*hostVol](nil)
	return c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	c.le.Info("starting plugin host")
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// shutdown all plugin instances when exiting
	defer c.pluginManifestTrackers.ClearContext()
	defer c.pluginInstances.ClearContext()
	defer c.hostPluginPlatformID.SetPromise(nil)

	// get the platform id
	pluginPlatformID, err := c.host.GetPlatformId(ctx)
	if err != nil {
		return err
	}
	c.le.Debugf("plugin host platform id is: %v", pluginPlatformID)
	c.hostPluginPlatformID.SetResult(pluginPlatformID, nil)

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

	// startup manifest watchers & plugin instances
	c.pluginManifestTrackers.SetContext(ctx, true)
	c.pluginInstances.SetContext(ctx, true)

	<-ctx.Done()
	return nil
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
		return directive.R(c.resolveLoadPlugin(ctx, inst, d))
	case bifrost_rpc.LookupRpcClient:
		return directive.R(bldr_plugin.ResolveLookupRpcClient(ctx, d, c))
	}
	return nil, nil
}

// AddPluginReference adds a reference to the plugin, returning the RunningPlugin
// handle and a release function.
//
// Returns nil, nil, err if any error occurs.
func (c *Controller) AddPluginReference(pluginID string) (bldr_plugin.RunningPluginRef, func()) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginID)
	return plg, ref.Release
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// buildPluginMux builds the rpc mux for plugins.
func (c *Controller) buildPluginMux(
	pluginID string,
	manifest *bldr_manifest.ManifestSnapshot,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	proxyHostVolInfo *volume.VolumeInfo,
	distFS,
	assetsFS *unixfs.FSHandle,
) srpc.Mux {
	busInvoker := bifrost_rpc.NewInvoker(c.bus, "plugin/"+pluginID, true)
	mux := srpc.NewMux(busInvoker) // busInvoker

	// register access host volume via rpc service
	_ = volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, proxyHostVol, bldr_plugin.HostVolumeServiceIDPrefix)

	// register access web views via bus service
	_ = web_view.SRPCRegisterAccessWebViews(mux, web_view_server.NewAccessWebViewsViaBus(c.le, c.bus))

	// register plugin host service
	_ = bldr_plugin.SRPCRegisterPluginHost(mux, bldr_plugin_host.NewPluginHostServer(c.bus, c.le, pluginID, manifest, proxyHostVolInfo))

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

	return mux
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

// _ is a type assertion
var (
	_ controller.Controller              = ((*Controller)(nil))
	_ bldr_plugin.LookupRpcClientHandler = ((*Controller)(nil))
)
