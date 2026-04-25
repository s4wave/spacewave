package resource_listener

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
)

// ControllerID is the controller identifier.
const ControllerID = "resource/listener"

// Version is the component version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "resource service unix socket listener controller"

// Controller is the resource listener controller.
type Controller struct {
	*bus.BusController[*Config]
}

// NewFactory constructs the component factory.
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
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
