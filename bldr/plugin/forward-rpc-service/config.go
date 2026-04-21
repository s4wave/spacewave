package bldr_plugin_forward_rpc_service

import (
	"regexp"

	"github.com/aperturerobotics/controllerbus/config"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/util/confparse"
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
		return bldr_plugin.ErrEmptyPluginID
	}
	if _, err := c.ParseServiceIdRe(); err != nil {
		return err
	}
	if err := c.GetBackoff().Validate(true); err != nil {
		return err
	}
	return nil
}

// SetServiceIdRe sets the service id regex.
func (c *Config) SetServiceIdRe(re string) {
	c.ServiceIdRe = re
}

// ParseServiceIdRe parses the service id regex.
// Returns nil if the field was empty.
func (c *Config) ParseServiceIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetServiceIdRe())
}

// SetServerIdRe sets the server id regex.
func (c *Config) SetServerIdRe(re string) {
	c.ServerIdRe = re
}

// ParseServerIdRe parses the server id regex.
// Returns nil if the field was empty.
func (c *Config) ParseServerIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetServerIdRe())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
