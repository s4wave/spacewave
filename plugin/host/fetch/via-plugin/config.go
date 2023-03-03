package plugin_fetch_viaplugin

import (
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_fetch "github.com/aperturerobotics/bldr/plugin/host/fetch"
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

// SetFetchPluginIdRegex sets the fetch_plugin_id regex.
func (c *Config) SetFetchPluginIdRegex(re string) {
	c.FetchPluginIdRegex = re
}

// ParseFetchPluginIdRegex parses the fetch_plugin_id regex.
// Returns nil if the field was empty.
func (c *Config) ParseFetchPluginIdRegex() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetFetchPluginIdRegex())
}

// _ is a type assertion
var _ plugin_fetch.Config = ((*Config)(nil))
