package bldr_plugin_handle_web_view

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

// Validate checks the ApplyBucketConfig.
func (c *Config) Validate() error {
	if c.GetPluginId() == "" {
		return plugin.ErrEmptyPluginID
	}
	if _, err := c.ParseWebViewIdRegex(); err != nil {
		return err
	}
	return nil
}

// SetWebViewIdRegex sets the web view id regex.
func (c *Config) SetWebViewIdRegex(re string) {
	c.WebViewIdRegex = re
}

// ParseWebViewIdRegex parses the handle web view id regex.
// Returns nil if the field was empty.
func (c *Config) ParseWebViewIdRegex() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetWebViewIdRegex())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
