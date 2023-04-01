package bldr_plugin_forward_rpc_service

import (
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/config"
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

// Validate checks the config.
func (c *Config) Validate() error {
	if c.GetPluginId() == "" {
		return plugin.ErrEmptyPluginID
	}
	if _, err := c.ParseServiceIdRegex(); err != nil {
		return err
	}
	if err := c.GetBackoff().Validate(true); err != nil {
		return err
	}
	return nil
}

// SetServiceIdRegex sets the service id regex.
func (c *Config) SetServiceIdRegex(re string) {
	c.ServiceIdRegex = re
}

// ParseServiceIdRegex parses the service id regex.
// Returns nil if the field was empty.
func (c *Config) ParseServiceIdRegex() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetServiceIdRegex())
}

// SetServerIdRegex sets the server id regex.
func (c *Config) SetServerIdRegex(re string) {
	c.ServerIdRegex = re
}

// ParseServerIdRegex parses the server id regex.
// Returns nil if the field was empty.
func (c *Config) ParseServerIdRegex() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetServerIdRegex())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
