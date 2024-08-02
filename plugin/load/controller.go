package bldr_plugin_load

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/plugin/load"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

var controllerDescrip = "load plugin"

// Controller creates a LoadPlugin directive.
type Controller struct {
	*bus.BusController[*Config]
}

// NewController constructs a new load plugin controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) (*Controller, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return &Controller{
		BusController: bus.NewBusController(
			le,
			b,
			conf,
			ControllerID,
			Version,
			controllerDescrip,
		),
	}, nil
}

// NewFactory constructs a new load plugin controller.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		NewConfig,
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
			}, nil
		},
	)
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	pluginIDs := c.GetConfig().CleanupPluginIds()
	for _, pluginID := range pluginIDs {
		_, dirRef, err := c.GetBus().AddDirective(
			bldr_plugin.NewLoadPlugin(
				pluginID,
			),
			nil,
		)
		if err != nil {
			return err
		}
		defer dirRef.Release()
	}
	<-ctx.Done()
	return context.Canceled
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
