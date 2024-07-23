package plugin_host_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_server "github.com/aperturerobotics/bldr/web/view/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_server "github.com/aperturerobotics/hydra/unixfs/rpc/server"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/sirupsen/logrus"
)

// Controller implements the PluginHost controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// info contains the controller info
	info *controller.Info
	// host is the plugin host
	host bldr_plugin_host.PluginHost
	// hostClient is a loopback srpc client to the host.
	hostClient srpc.Client
	// hostPluginPlatformID is the plugin platform ID to use.
	hostPluginPlatformID *promise.PromiseContainer[string]
	// objKey is the PluginHost object key (from the config)
	objKey string
	// peerID is the parsed peer id for sending world ops
	peerID peer.ID
	// peerIDStr is the parsed peer id string
	peerIDStr string
	// objLoop is the object watcher loop
	// watches the PluginHost object
	objLoop *world_control.WatchLoop
	// worldStateCtr contains the world state handle
	worldStateCtr *ccontainer.CContainer[world.WorldState]
	// hostVolumeCtr is a container with the host volume.
	hostVolumeCtr *ccontainer.CContainer[*hostVol]
	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.KeyedRefCount[string, *executePlugin]
	// downloadManifests manages fetching plugin manifests.
	// key: plugin ID
	// controlled by pluginInstances
	downloadManifests *keyed.KeyedRefCount[string, *downloadManifest]
	// watchFetchManifests manages watching FetchManifest directives.
	// key: plugin ID
	// controlled by pluginInstances
	watchFetchManifests *keyed.KeyedRefCount[string, *watchFetchManifest]
	// pluginManifestWatcher manages watching any matched PluginManifest.
	// key: objKey of matched PluginManifest
	// controlled by pluginInstances
	pluginManifestWatcher *keyed.Keyed[string, *watchWorldManifest]
	// rmtx guards below fields
	rmtx sync.RWMutex
	// pluginManifests contains the latest known manifest objKey for the loaded plugins.
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
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:                   le,
		bus:                  bus,
		conf:                 conf,
		info:                 info,
		host:                 host,
		hostPluginPlatformID: promise.NewPromiseContainer[string](),
		objKey:               conf.GetObjectKey(),
		peerID:               peerID,
		peerIDStr:            peerID.String(),
		pluginManifests:      make(map[string]*bldr_manifest.ManifestSnapshot),
		worldStateCtr:        ccontainer.NewCContainer[world.WorldState](nil),
		hostVolumeCtr:        ccontainer.NewCContainer[*hostVol](nil),
	}
	c.pluginManifestWatcher = keyed.NewKeyedWithLogger(c.newWatchWorldManifest, le.WithField("tracker", "manifest-watcher"))
	c.downloadManifests = keyed.NewKeyedRefCountWithLogger(c.newDownloadManifest, le.WithField("tracker", "manifest-downloader"))
	c.watchFetchManifests = keyed.NewKeyedRefCountWithLogger(c.newWatchFetchManifest, le.WithField("tracker", "fetch-manifest-watcher"))
	c.pluginInstances = keyed.NewKeyedRefCountWithLogger(c.newRunningPlugin, le.WithField("tracker", "plugin-instances"))
	c.objLoop = world_control.NewWatchLoop(
		le.WithField("control-loop", "plugin-host-controller"),
		c.objKey,
		c.ProcessState,
	)
	c.hostClient = srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(bifrost_rpc.NewInvoker(bus, "plugin-host", true))))
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
	defer c.pluginManifestWatcher.ClearContext()
	defer c.pluginInstances.ClearContext()
	defer c.downloadManifests.ClearContext()
	defer c.hostPluginPlatformID.SetPromise(nil)

	// get the platform id
	pluginPlatformID, err := c.host.GetPlatformId(ctx)
	if err != nil {
		return err
	}
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

	// construct the world engine handle
	busEngine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	ws := world.NewEngineWorldState(busEngine, true)
	c.worldStateCtr.SetValue(ws)
	defer c.worldStateCtr.SetValue(ws)

	// run initial cleanup
	if err := c.cleanupUnknownPlugins(ctx, ws, pluginPlatformID); err != nil {
		return err
	}

	// startup manifest watchers & plugin instances
	c.pluginManifestWatcher.SetContext(ctx, true)
	c.pluginInstances.SetContext(ctx, true)
	c.downloadManifests.SetContext(ctx, true)
	c.watchFetchManifests.SetContext(ctx, true)

	// watch the plugin host for changes
	return c.objLoop.Execute(ctx, ws)
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
	c.rmtx.Lock()
	defer c.rmtx.Unlock()
	ref, plg, _ := c.pluginInstances.AddKeyRef(pluginID)
	var downloadRef *keyed.KeyedRef[string, *downloadManifest]
	var watchFetchRef *keyed.KeyedRef[string, *watchFetchManifest]
	downloadRef, _, _ = c.downloadManifests.AddKeyRef(pluginID)
	if c.conf.GetWatchFetchManifest() {
		watchFetchRef, _, _ = c.watchFetchManifests.AddKeyRef(pluginID)
	}
	return plg, func() {
		ref.Release()
		downloadRef.Release()
		if watchFetchRef != nil {
			watchFetchRef.Release()
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
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

// buildPluginMux builds the rpc mux for plugins.
func (c *Controller) buildPluginMux(
	pluginID string,
	manifest *bldr_manifest.ManifestSnapshot,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	proxyHostVolInfo *volume.VolumeInfo,
	distFS,
	assetsFS *unixfs.FSHandle,
) srpc.Mux {
	mux := srpc.NewMux()

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

// buildWorldState builds the world state handle.
func (c *Controller) getWorldState(ctx context.Context) (world.WorldState, error) {
	return c.worldStateCtr.WaitValue(ctx, nil)
}

// cleanupUnknownPlugins calls DeletePlugin for any plugins without a matching manifest.
func (c *Controller) cleanupUnknownPlugins(ctx context.Context, ws world.WorldState, filterPlatformID string) error {
	// fetch all known plugin manifests
	pluginManifests, pluginManifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, filterPlatformID, c.objKey)
	if err != nil {
		return err
	}
	for _, err := range pluginManifestErrs {
		c.le.WithError(err).Warn("ignoring invalid plugin manifest")
	}

	// list ids from the plugin host
	loadedPlugins, err := c.host.ListPlugins(ctx)
	if err != nil {
		return err
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
		if err := c.host.DeletePlugin(ctx, unknownPlugin); err != nil {
			if err == context.Canceled {
				return err
			}
			c.le.WithError(err).Warnf("unable to clear old plugin: %s", unknownPlugin)
		}
	}

	return nil
}

// syncWatchPluginManifests starts/stop routines to watch the plugin manifests.
func (c *Controller) syncWatchPluginManifests(manifestObjKeys []string) {
	c.pluginManifestWatcher.SyncKeys(manifestObjKeys, true)
}

// _ is a type assertion
var (
	_ controller.Controller                 = ((*Controller)(nil))
	_ bldr_plugin.LookupRpcClientHandler    = ((*Controller)(nil))
	_ bldr_plugin_host.PluginHostController = ((*Controller)(nil))
)
