package volume_bolt

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetPath() == "" {
		return errors.New("path must be specified")
	}
	if err := c.GetKvKeyOpts().Validate(); err != nil {
		return err
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return proto.Equal(c, ot)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
