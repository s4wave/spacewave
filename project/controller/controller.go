package bldr_project_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bldr/project"

// Controller is the bldr Project controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the controller config
	c *Config
	// manifestBuilders is the set of keyed manifest-id build controllers.
	// NOTE: this will eventually be replaced with Forge jobs.
	manifestBuilders *keyed.KeyedRefCount[string, *manifestBuilderTracker]
	// TODO distBuilders
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	ctrl := &Controller{
		le:  le,
		bus: bus,
		c:   cc,
	}
	ctrl.manifestBuilders = keyed.NewKeyedRefCountWithLogger(ctrl.newManifestBuilderTracker, le)
	return ctrl
}

// GetConfig returns the config.
func (c *Controller) GetConfig() *Config {
	return c.c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr project controller",
	)
}

// AddManifestBuilderRef adds a reference to a manifest compiler.
func (c *Controller) AddManifestBuilderRef(meta *bldr_manifest.ManifestMeta) *ManifestBuilderRef {
	metaB58 := meta.MarshalB58()
	ref, tracker, _ := c.manifestBuilders.AddKeyRef(metaB58)
	return newManifestBuilderRef(ref, tracker)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// start the startup plugins and config set if configured.
	projConf := c.c.GetProjectConfig()

	// start the plugin build controllers
	c.manifestBuilders.SetContext(ctx, true)
	defer c.manifestBuilders.SetContext(nil, false)

	// load all initial plugins, if configured
	loadPluginIDs := projConf.GetStart().GetPlugins()
	if c.c.GetStartProject() && len(loadPluginIDs) != 0 {
		for _, pluginID := range loadPluginIDs {
			c.le.WithField("plugin-id", pluginID).Info("loading startup plugin")
			_, plugRef, err := c.bus.AddDirective(plugin.NewLoadPlugin(pluginID), nil)
			if err != nil {
				return err
			}
			defer plugRef.Release()
		}
	}

	// wait for context cancel
	<-ctx.Done()

	// we release everything on return
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case plugin.LoadPlugin:
		return directive.R(c.resolveLoadPlugin(ctx, di, d), nil)
	case bldr_manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, di, d), nil)
	}

	return nil, nil
}

// BuildWorldEngine builds the world engine handle from the configured engine id.
func (c *Controller) BuildWorldEngine(ctx context.Context) *world.BusEngine {
	return world.NewBusEngine(ctx, c.bus, c.c.GetEngineId())
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
