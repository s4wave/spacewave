package plugin_fetch

import (
	"regexp"

	"github.com/aperturerobotics/bldr/plugin"
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

// Validate checks the ApplyBucketConfig.
func (c *Config) Validate() error {
	if c.GetPluginId() == "" {
		return plugin.ErrEmptyPluginID
	}
	if _, err := c.ParseFetchPluginIdRegex(); err != nil {
		return err
	}
	return nil
}

// ParseFetchPluginIdRegex parses the fetch_plugin_id regex.
// Returns nil if the field was empty.
func (c *Config) ParseFetchPluginIdRegex() (*regexp.Regexp, error) {
	r := c.GetFetchPluginIdRegex()
	if r == "" {
		return nil, nil
	}
	return regexp.Compile(r)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
