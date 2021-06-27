package forge_kvtx

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	for i, op := range c.GetOps() {
		if err := op.Validate(); err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}
	}
	if inpName := c.GetConfigInput(); inpName != "" {
		if err := checkReservedName(inpName); err != nil {
			return errors.Wrap(err, "config_input")
		}
	}
	return nil
}

// IsEmpty checks if there are no operations in the config.
func (c *Config) IsEmpty() bool {
	var any bool
	for _, op := range c.GetOps() {
		if !op.IsEmpty() {
			any = true
			break
		}
	}
	return !any
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return proto.Equal(c, other)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (c *Config) MarshalBlock() ([]byte, error) {
	return proto.Marshal(c)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (c *Config) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, c)
}

// _ is a type assertion
var (
	_ config.Config = ((*Config)(nil))
	_ block.Block   = ((*Config)(nil))
)
