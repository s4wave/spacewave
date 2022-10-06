package bldr_project_controller

import (
	"context"

	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/blang/semver"
	"github.com/pkg/errors"
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
	// pluginBuilders is the set of keyed plugin-id build controllers.
	pluginBuilders *keyed.KeyedRefCount[*pluginBuilderTracker]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	ctrl := &Controller{
		le:  le,
		bus: bus,
		c:   cc,
	}
	ctrl.pluginBuilders = keyed.NewKeyedRefCount(ctrl.newPluginBuilderTracker)
	return ctrl
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr project controller",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// start the startup plugins and config set if configured.
	projConf := c.c.GetProjectConfig()
	if c.c.GetStartProject() {
		configSet, err := projConf.GetStart().ResolveConfigSet(ctx, c.bus)
		if err != nil {
			if err == context.Canceled {
				return err
			}
			return errors.Wrap(err, "unable to resolve start: config set")
		}

		_, csRef, err := c.bus.AddDirective(configset.NewApplyConfigSet(configSet), nil)
		if err != nil {
			return err
		}
		defer csRef.Release()
	}

	// load all initial plugins, if configured
	startPluginIDs := projConf.GetStart().GetLoadPluginIds()
	if c.c.GetStartProject() && len(startPluginIDs) != 0 {
		for _, pluginID := range startPluginIDs {
			_, plugRef, err := c.bus.AddDirective(plugin_host.NewLoadPlugin(pluginID), nil)
			if err != nil {
				return err
			}
			defer plugRef.Release()
		}
	}

	// start the plugin build controllers
	c.pluginBuilders.SetContext(ctx, true)
	defer c.pluginBuilders.SetContext(nil, false)

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
) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case plugin_host.LoadPlugin:
		return c.resolveLoadPlugin(ctx, di, d), nil
	}

	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
