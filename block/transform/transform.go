package block_transform

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
)

// StepFactory constructs transform steps.
type StepFactory interface {
	// GetConfigID returns the unique config ID for the transform step.
	GetConfigID() string
	// ConstructConfig constructs an instance of the transform configuration.
	ConstructConfig() config.Config
	// Construct constructs the associated transform step given configuration.
	Construct(config.Config, controller.ConstructOpts) (Step, error)
}

// Step implements a constructed transform step.
type Step interface {
	// EncodeBlock encodes the block according to the config.
	// May reuse the same byte slice if possible.
	EncodeBlock([]byte) ([]byte, error)
	// DecodeBlock decodes the block according to the config.
	// May reuse the same byte slice if possible.
	DecodeBlock([]byte) ([]byte, error)
}
