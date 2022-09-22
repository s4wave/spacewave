package plugin_host_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
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
	// objKey is the PluginHost object key (from the config)
	objKey string
	// peerID is the parsed peer id
	peerID peer.ID
	// peerIDStr is the parsed peer id string
	peerIDStr string
	// objLoop is the object watcher loop
	// watches the PluginHost object
	objLoop *world_control.ObjectLoop
	// pluginManifestWatcher manages watching any matched PluginManifest.
	// key: objKey of matched PluginManifest
	pluginManifestWatcher *keyed.Keyed[*pluginManifestTracker]
	// pluginInstances manages the list of running plugins by plugin ID.
	// key: plugin ID
	pluginInstances *keyed.Keyed[*runningPlugin]
	// rmtx guards below fields
	rmtx sync.RWMutex
	// pluginManifests contains the latest known manifest for the loaded plugins.
	pluginManifests map[string]*plugin.PluginManifest
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
		le:              le,
		bus:             bus,
		conf:            conf,
		info:            info,
		host:            host,
		objKey:          conf.GetObjectKey(),
		peerID:          peerID,
		peerIDStr:       peerID.Pretty(),
		pluginManifests: make(map[string]*plugin.PluginManifest),
	}
	c.pluginManifestWatcher = keyed.NewKeyed(c.newPluginManifestTracker)
	c.pluginInstances = keyed.NewKeyed(c.newRunningPlugin)
	c.objLoop = world_control.NewObjectLoop(
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
func (c *Controller) Execute(rctx context.Context) error {
	c.le.Info("starting native process plugin host")
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// construct the world engine
	busEngine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	defer busEngine.Close()
	ws := world.NewEngineWorldState(ctx, busEngine, true)

	// run initial cleanup
	if err := c.cleanupUnknownPlugins(ctx, ws); err != nil {
		return err
	}

	// watch the plugin host for changes
	c.pluginManifestWatcher.SetContext(ctx, true)
	return c.objLoop.Execute(ctx, ws)
}

// cleanupUnknownPlugins calls DeletePlugin for any plugins without a matching manifest.
func (c *Controller) cleanupUnknownPlugins(ctx context.Context, ws world.WorldState) error {
	// fetch all known plugin manifests
	pluginManifests, _, pluginManifestErrs, err := plugin_host.CollectPluginHostPluginManifests(ctx, ws, c.objKey)
	if err != nil {
		return err
	}
	for _, err := range pluginManifestErrs {
		c.le.WithError(err).Warn("ignoring invalid plugin manifest")
	}

	// build map of known ids
	knownPluginIDs := make(map[string]struct{})
	for _, manifest := range pluginManifests {
		if id := manifest.GetPluginId(); id != "" {
			knownPluginIDs[id] = struct{}{}
		}
	}

	// list ids from the plugin host
	loadedPlugins, err := c.host.ListPlugins(ctx)
	if err != nil {
		return err
	}

	// delete any unknowns
	var unknownPlugins []string
	for _, loadedPlugin := range loadedPlugins {
		if _, ok := knownPluginIDs[loadedPlugin]; !ok {
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
) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
