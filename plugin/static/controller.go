package plugin_static

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/peer"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
)

// Controller is the static plugin loader controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the controller config
	conf *Config
	// info is the controller info
	info *controller.Info
	// plugin is the static plugin to load on startup
	plugin *StaticPlugin
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	cc *Config,
	info *controller.Info,
	plugin *StaticPlugin,
) *Controller {
	return &Controller{
		le:     le,
		bus:    bus,
		conf:   cc,
		info:   info,
		plugin: plugin,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the given controller.
func (c *Controller) Execute(ctx context.Context) error {
	rplugin := c.plugin
	if rplugin == nil {
		return nil
	}

	manifest := rplugin.Manifest
	pluginID := manifest.GetPluginId()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if rplugin.PluginDistFs == nil {
		return errors.New("plugin dist fs must be set")
	}

	// build world state
	busEngine := world.NewBusEngine(ctx, c.bus, c.conf.GetEngineId())
	defer busEngine.Close()
	ws := world.NewEngineWorldState(ctx, busEngine, true)

	// wait for the plugin host to exist
	pluginHostKey := c.conf.GetPluginHostKey()
	_, err := world_control.WaitForObjectRev(ctx, c.le, ws, pluginHostKey, 0)
	if err != nil {
		return err
	}

	// lookup static plugin in world
	existingManifest, _, err := plugin_host.LookupPluginHostManifest(ctx, ws, pluginHostKey, pluginID)
	if err != nil {
		return err
	}
	if existingManifest == nil {
		le := c.le.WithField("plugin-id", pluginID)
		peerID, err := c.conf.ParsePeerID()
		if err == nil && len(peerID) == 0 {
			err = peer.ErrEmptyPeerID
		}
		if err != nil {
			return err
		}

		le.Debug("copying static plugin to storage")
		ts := timestamp.Now()
		fsManifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
			return rplugin.CreatePluginManifest(ctx, bcs, &ts)
		})
		if err != nil {
			return err
		}

		le.Debug("loading static plugin to plugin host")
		err = plugin_host.UpdatePluginManifest(ctx, ws, peerID, pluginHostKey, pluginID, fsManifestRef)
		if err != nil {
			return err
		}

		le.Info("loaded plugin to world successfully")
	}

	// if disable_load_plugin is set, exit successfully.
	if c.conf.GetDisableLoadPlugin() {
		return nil
	}

	// create LoadPlugin directive
	return plugin_host.ExLoadPlugin(ctx, c.bus, pluginID, nil)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
