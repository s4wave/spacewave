package downstream_core

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

const controllerID = "bldr/e2e/downstreamapp/core"

type Config struct{}

func (c *Config) Validate() error { return nil }

func (c *Config) GetConfigID() string { return controllerID }

func (c *Config) EqualsConfig(other config.Config) bool {
	_, ok := other.(*Config)
	return ok
}

func (c *Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct{}{})
}

func (c *Config) UnmarshalJSON([]byte) error {
	return nil
}

func (c *Config) MarshalVT() ([]byte, error) {
	return nil, nil
}

func (c *Config) MarshalToVT([]byte) (int, error) {
	return 0, nil
}

func (c *Config) MarshalToSizedBufferVT([]byte) (int, error) {
	return 0, nil
}

func (c *Config) UnmarshalVT(data []byte) error {
	if len(data) != 0 {
		return errors.Errorf("unexpected downstream core config payload length %d", len(data))
	}
	return nil
}

func (c *Config) SizeVT() int {
	return 0
}

func (c *Config) Reset() {}

func (c *Config) ResetVT() {}

func (c *Config) ReturnToVTPool() {}

type Controller struct {
	*bus.BusController[*Config]
}

func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		controllerID,
		controllerID,
		controller.MustParseVersion("0.0.1"),
		"downstream e2e core controller",
		func() *Config { return &Config{} },
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{BusController: base}, nil
		},
	)
}

func (c *Controller) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

var _ config.Config = (*Config)(nil)
var _ controller.Controller = (*Controller)(nil)
