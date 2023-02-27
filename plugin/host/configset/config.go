package plugin_host_configset

import (
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate checks the ApplyBucketConfig.
func (c *Config) Validate() error {
	csm := configset_proto.ConfigSetMap(c.GetConfigSet())
	if err := csm.Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
