package volume_sqlite

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// NewConfig constructs the configuration with the path and table.
func NewConfig(path string, table string) *Config {
	return &Config{
		Path:  path,
		Table: table,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetPath() == "" {
		return errors.New("path must be specified")
	}
	if c.GetTable() == "" {
		return errors.New("table must be specified")
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
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
