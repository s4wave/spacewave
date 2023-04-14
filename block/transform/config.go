package block_transform

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// NewConfig constructs a new config with a set of underlying steps.
func NewConfig(steps []config.Config) (*Config, error) {
	c := &Config{}
	for _, step := range steps {
		sc, err := NewStepConfig(step)
		if err != nil {
			return nil, err
		}
		c.Steps = append(c.Steps, sc)
	}
	return c, nil
}

// NewTransformConfigBlock is a transform configuration block constructor.
func NewTransformConfigBlock() block.Block {
	return &Config{}
}

// Clone clones the block transform config.
func (c *Config) Clone() *Config {
	return c.CloneVT()
}

// Validate performs cursory validation of the config.
func (c *Config) Validate() error {
	for i, s := range c.GetSteps() {
		if err := s.Validate(); err != nil {
			return errors.Errorf(
				"step[%d]: config invalid: %s",
				i,
				err.Error(),
			)
		}
	}
	return nil
}

// GetEmpty returns if the transform config is empty.
func (c *Config) GetEmpty() bool {
	return len(c.GetSteps()) == 0
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *Config) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *Config) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (c *Config) ApplySubBlock(id uint32, next block.SubBlock) error {
	// no-op
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (c *Config) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[1] = NewStepConfigSetSubBlockCtor(&c.Steps)(false)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (c *Config) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return NewStepConfigSetSubBlockCtor(&c.Steps)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Config)(nil))
	_ block.BlockWithSubBlocks = ((*Config)(nil))
)
