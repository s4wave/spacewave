package transform_s2

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
)

// ConfigID is the configuration identifier.
const ConfigID = "hydra/transform/s2/1"

// Factory constructs the transform step.
type Factory struct {
}

// NewFactory constructs the factory object.
func NewFactory() *Factory {
	return &Factory{}
}

// GetConfigID returns the unique config ID for the transform step.
func (f *Factory) GetConfigID() string {
	return ConfigID
}

// ConstructConfig constructs an instance of the transform configuration.
func (f *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated transform step given configuration.
func (f *Factory) Construct(
	conf config.Config, opts controller.ConstructOpts,
) (block_transform.Step, error) {
	c := conf.(*Config)
	return NewS2(c)
}

// ConstructMockConfig constructs an instance of the transform configuration for testing.
func (f *Factory) ConstructMockConfig() []config.Config {
	return []config.Config{&Config{}}
}

// _ is a type assertion
var _ block_transform.StepFactory = ((*Factory)(nil))
