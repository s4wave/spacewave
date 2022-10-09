package bldr_example

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "bldr/example/demo"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "demo controller"

// Demo is a demo controller.
type Demo struct {
	*bus.BusController[*Config]
}

// NewDemo constructs a new demo controller.
func NewDemo(le *logrus.Entry, b bus.Bus, conf *Config) (*Demo, error) {
	return &Demo{
		BusController: bus.NewBusController(le, b, conf, ControllerID, Version, controllerDescrip),
	}, nil
}

// NewFactory constructs the demo controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Demo, error) {
			return &Demo{BusController: base}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (d *Demo) Execute(ctx context.Context) error {
	le := d.GetLogger()
	le.Info("hello from the bldr example demo controller")
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Demo)(nil)
