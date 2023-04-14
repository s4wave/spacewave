package block_transform

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"google.golang.org/protobuf/proto"
)

// NewStepConfig constructs the step config with a underlying config.
func NewStepConfig(conf config.Config) (*StepConfig, error) {
	dat, err := proto.Marshal(conf)
	if err != nil {
		return nil, err
	}
	return &StepConfig{
		Id:     conf.GetConfigID(),
		Config: dat,
	}, nil
}

// IsNil returns if the object is nil.
func (c *StepConfig) IsNil() bool {
	return c == nil
}

// Validate performs cursory validation of the config.
func (c *StepConfig) Validate() error {
	if id := c.GetId(); len(id) == 0 {
		return errors.New("step id cannot be nil")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *StepConfig) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *StepConfig) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*StepConfig)(nil))
