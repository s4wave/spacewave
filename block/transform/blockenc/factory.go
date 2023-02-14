package transform_blockenc

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/util/blockenc"
)

// ConfigID is the configuration identifier.
const ConfigID = "hydra/transform/blockenc"

// StepFactory constructs the transform step.
type StepFactory struct {
}

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

// ConstructMockConfig constructs an instance of the transform configuration for testing.
func (f *StepFactory) ConstructMockConfig() []config.Config {
	// random 32 byte key
	key := make([]byte, 32)
	_, _ = rand.Reader.Read(key)
	var confs []config.Config
	for i := blockenc.BlockEnc_BlockEnc_NONE; i <= blockenc.BlockEnc_BlockEnc_MAX; i++ {
		confs = append(confs, &Config{
			BlockEnc: i,
			Key:      key,
		})
	}
	return confs
}

// Construct constructs the associated transform step given configuration.
func (f *StepFactory) Construct(
	conf config.Config, opts controller.ConstructOpts,
) (block_transform.Step, error) {
	c := conf.(*Config)
	return NewBlockEnc(c)
}

// _ is a type assertion
var _ block_transform.StepFactory = ((*StepFactory)(nil))
