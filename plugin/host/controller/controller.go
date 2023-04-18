package plugin_host_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_server "github.com/aperturerobotics/bldr/web/view/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/starpc/echo"
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
	host plugin_host.PluginHost
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
	// hostVolumeCtr is a container with the host volume.
	hostVolumeCtr *ccontainer.CContainer[*hostVol]
	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.KeyedRefCount[string, *runningPlugin]
	// pluginManifestFetchers manages fetching plugin manifests.
	// key: plugin ID
	// controlled by pluginInstances
	pluginManifestFetchers *keyed.KeyedRefCount[string, *pluginManifestFetcher]
	// pluginManifestWatcher manages watching any matched PluginManifest.
	// key: objKey of matched PluginManifest
	// controlled by pluginInstances
	pluginManifestWatcher *keyed.Keyed[string, *pluginManifestTracker]
	// rmtx guards below fields
	rmtx sync.RWMutex
	// pluginManifests contains the latest known manifest objKey for the loaded plugins.
	pluginManifests map[string]pluginManifestSnapshot
}

// hostVol contains a snapshot of the host volume.
type hostVol struct {
	vol  volume.Volume
	info *volume.VolumeInfo
}

// pluginManifestSnapshot contains a snapshot of a plugin manifest.
type pluginManifestSnapshot struct {
	objKey      string
	manifest    *bldr_manifest.Manifest
	manifestRef *bucket.ObjectRef
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	info *controller.Info,
	host plugin_host.PluginHost,
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
		peerIDStr:            peerID.Pretty(),
		pluginManifests:      make(map[string]pluginManifestSnapshot),
		hostVolumeCtr:        ccontainer.NewCContainer[*hostVol](nil),
	}
	c.pluginManifestWatcher = keyed.NewKeyedWithLogger(c.newPluginManifestTracker, le.WithField("tracker", "manifest-watcher"))
	c.pluginManifestFetchers = keyed.NewKeyedRefCountWithLogger(c.newPluginManifestFetcher, le.WithField("tracker", "manifest-fetcher"))
	c.pluginInstances = keyed.NewKeyedRefCountWithLogger(c.newRunningPlugin, le.WithField("tracker", "plugin-instances"))
	c.objLoop = world_control.NewWatchLoop(
		le.WithField("control-loop", "plugin-host-controller"),
		c.objKey,
		c.ProcessState,
	)
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
	defer c.pluginManifestFetchers.ClearContext()
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
		volume_rpc_server.Version, "proxy to plugin host volume",
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
	ws, wsRel := c.buildWorldState(ctx)
	defer wsRel()

	// run initial cleanup
	if err := c.cleanupUnknownPlugins(ctx, ws, pluginPlatformID); err != nil {
		return err
	}

	// startup manifest watchers & plugin instances
	c.pluginManifestWatcher.SetContext(ctx, true)
	c.pluginInstances.SetContext(ctx, true)
	c.pluginManifestFetchers.SetContext(ctx, true)

	// watch the plugin host for changes
	return c.objLoop.Execute(ctx, ws)
}

// buildWorldState builds the world state handle.
// returns the release function
func (c *Controller) buildWorldState(ctx context.Context) (world.WorldState, func()) {
	busEngine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	return world.NewEngineWorldState(ctx, busEngine, true), busEngine.Close
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

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context tasked is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case plugin.LoadPlugin:
		return directive.R(c.resolveLoadPlugin(ctx, inst, d))
	}
	return nil, nil
}

// AddPluginReference adds a reference to the plugin, returning the RunningPlugin
// handle and a release function.
//
// Returns nil, nil, err if any error occurs.
func (c *Controller) AddPluginReference(pluginID string) (plugin.RunningPlugin, func()) {
	c.rmtx.Lock()
	defer c.rmtx.Unlock()
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
	manifest pluginManifestSnapshot,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	proxyHostVolInfo *volume.VolumeInfo,
) srpc.Mux {
	// busInvoker := bifrost_rpc.NewInvoker(c.bus, "plugin/"+pluginID)
	mux := srpc.NewMux() // busInvoker

	// register access host volume via rpc service
	_ = volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, proxyHostVol, plugin.HostVolumeServiceIDPrefix)

	// register access web views via bus service
	_ = web_view.SRPCRegisterAccessWebViews(mux, web_view_server.NewAccessWebViewsViaBus(c.le, c.bus))

	// register plugin host service
	_ = plugin.SRPCRegisterPluginHost(mux, newPluginHostServer(c, pluginID, manifest, proxyHostVolInfo))

	// register echoer (sanity test) service
	_ = echo.SRPCRegisterEchoer(mux, echo.NewEchoServer(nil))

	return mux
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
