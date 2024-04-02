package transform_chksum

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
)

// ConfigID is the configuration identifier.
const ConfigID = "hydra/transform/chksum"

// StepFactory constructs the transform step.
type StepFactory struct{}

// NewStepFactory constructs the factory object.
func NewStepFactory() *StepFactory {
	return &StepFactory{}
}

// GetConfigID returns the unique config ID for the transform step.
func (f *StepFactory) GetConfigID() string {
	return ConfigID
}

// ConstructConfig constructs an instance of the transform configuration.
func (f *StepFactory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated transform step given configuration.
func (f *StepFactory) Construct(
	conf config.Config, opts controller.ConstructOpts,
) (block_transform.Step, error) {
	c := conf.(*Config)
	return NewChksum(c)
}

// ConstructMockConfig constructs an instance of the transform configuration for testing.
func (f *StepFactory) ConstructMockConfig() []config.Config {
	return []config.Config{&Config{}}
}

// _ is a type assertion
var _ block_transform.StepFactory = ((*StepFactory)(nil))
