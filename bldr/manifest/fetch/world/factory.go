package manifest_fetch_world

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// NewFactory builds the controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusFactory(
		b,
		ConfigID,
		Version,
		func() *Config { return &Config{} },
		func(le *logrus.Entry, b bus.Bus, conf *Config) (controller.Controller, error) {
			if err := conf.Validate(); err != nil {
				return nil, err
			}
			return NewController(le, b, conf), nil
		},
	)
}
