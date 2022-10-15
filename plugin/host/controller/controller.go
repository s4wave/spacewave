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
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
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
	// peerID is the parsed peer id for sending world ops
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
	// pluginRefs tracks the references for each plugin.
	pluginRefs map[string][]*pluginReference
	// pluginManifests contains the latest known manifest objKey for the loaded plugins.
	pluginManifests map[string]pluginManifestSnapshot
}

// pluginReference is an open reference to a Plugin.
type pluginReference struct {
	// cb is the plugin status callback.
	// if an error is returned, removes the reference.
	// can be nil
	cb func(status *plugin_host.PluginStateSnapshot) error
	// removed is called when the reference is removed
	// can be nil
	removed func(err error)
}

// callCb calls the callback or calls remove with the error.
func (r *pluginReference) callCb(status *plugin_host.PluginStateSnapshot) error {
	if r.cb == nil {
		return nil
	}
	err := r.cb(status)
	if err != nil {
		r.removed(err)
	}
	return err
}

// pluginManifestSnapshot contains a snapshot of a plugin manifest.
type pluginManifestSnapshot struct {
	objKey      string
	manifest    *plugin.PluginManifest
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
		le:              le,
		bus:             bus,
		conf:            conf,
		info:            info,
		host:            host,
		objKey:          conf.GetObjectKey(),
		peerID:          peerID,
		peerIDStr:       peerID.Pretty(),
		pluginRefs:      make(map[string][]*pluginReference),
		pluginManifests: make(map[string]pluginManifestSnapshot),
	}
	c.pluginManifestWatcher = keyed.NewKeyedWithLogger(c.newPluginManifestTracker, le)
	c.pluginInstances = keyed.NewKeyedWithLogger(c.newRunningPlugin, le)
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
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	c.le.Info("starting native process plugin host")
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// release all plugin refs
	defer func() {
		perr := rerr
		if perr == nil {
			perr = context.Canceled
		}
		c.rmtx.Lock()
		for pluginID, pluginRefs := range c.pluginRefs {
			for _, ref := range pluginRefs {
				if ref != nil && ref.removed != nil {
					ref.removed(perr)
				}
			}
			delete(c.pluginRefs, pluginID)
		}
		c.pluginInstances.SyncKeys(nil, false)
		c.rmtx.Unlock()
	}()

	// construct the world engine handle
	ws, wsRel := c.buildWorldState(ctx)
	defer wsRel()

	// run initial cleanup
	if err := c.cleanupUnknownPlugins(ctx, ws); err != nil {
		return err
	}

	// startup manifest watchers & plugin instances
	c.pluginManifestWatcher.SetContext(ctx, true)
	c.pluginInstances.SetContext(ctx, true)

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
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case plugin_host.LoadPlugin:
		return directive.R(c.resolveLoadPlugin(ctx, inst, d))
	}
	return nil, nil
}

// LoadPlugin runs a plugin, yielding PluginStatus snapshots.
// Adds a reference to the plugin, if it already is loaded.
// Returns if context is canceled.
func (c *Controller) LoadPlugin(
	ctx context.Context,
	pluginID string,
	cb func(ps *plugin_host.PluginStateSnapshot) error,
) error {
	removedCh := make(chan error, 1)
	nref := &pluginReference{
		cb: cb,
		removed: func(err error) {
			removedCh <- err
		},
	}

	c.rmtx.Lock()
	refs := c.pluginRefs[pluginID]
	refs = append(refs, nref)
	c.pluginRefs[pluginID] = refs
	_ = c.pluginInstances.SetKey(pluginID, true)
	c.rmtx.Unlock()

	var err error
	select {
	case <-ctx.Done():
		err = context.Canceled
	case err = <-removedCh:
	}

	c.rmtx.Lock()
	refs = c.pluginRefs[pluginID]
	for i, ref := range refs {
		if ref == nref {
			refs[i] = refs[len(refs)-1]
			refs[len(refs)-1] = nil
			refs = refs[:len(refs)-1]
			c.pluginRefs[pluginID] = refs
			break
		}
	}
	c.rmtx.Unlock()

	return err
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// callPluginRefCallbacks calls all callbacks for plugin references.
// removes plugin refs for which cb() returns an error
// expects caller to lock rmtx
func (c *Controller) callPluginRefCallbacks(pluginID string, status *plugin_host.PluginStateSnapshot) {
	refs := c.pluginRefs[pluginID]
	for i := 0; i < len(refs); i++ {
		ref := refs[i]
		err := ref.callCb(status)
		if err != nil {
			refs[i] = refs[len(refs)-1]
			refs[len(refs)-1] = nil
			refs = refs[:len(refs)-1]
			i--
		}
	}
	c.pluginRefs[pluginID] = refs
}

// buildPluginMux builds the rpc mux for plugins.
func (c *Controller) buildPluginMux(pluginID string, manifest pluginManifestSnapshot) srpc.Mux {
	mux := srpc.NewMux()

	// register plugin host service
	_ = plugin.SRPCRegisterPluginHost(mux, newPluginHostServer(c, pluginID, manifest))
	_ = echo.SRPCRegisterEchoer(mux, echo.NewEchoServer(nil))

	return mux
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
