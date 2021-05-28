package block_transform

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
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

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *StepConfig) MarshalBlock() ([]byte, error) {
	return proto.Marshal(c)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *StepConfig) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, c)
}

// _ is a type assertion
var _ block.Block = ((*StepConfig)(nil))
