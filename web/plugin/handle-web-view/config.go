package bldr_web_plugin_handle_web_view

import (
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
	plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_web_plugin "github.com/aperturerobotics/bldr/web/plugin"
	"github.com/aperturerobotics/controllerbus/config"
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

// Validate checks the config.
func (c *Config) Validate() error {
	if c.GetWebPluginId() == "" {
		return errors.Wrap(plugin.ErrEmptyPluginID, "web_plugin_id")
	}
	if c.GetHandlePluginId() == "" {
		return errors.Wrap(plugin.ErrEmptyPluginID, "handle_plugin_id")
	}
	if _, err := c.ParseWebViewIdRe(); err != nil {
		return err
	}
	return nil
}

// ParseWebViewIdRe parses the handle web view id regex.
// Returns nil if the field was empty.
func (c *Config) ParseWebViewIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetWebViewIdRe())
}

// ToRequest converts the config into a request.
func (c *Config) ToRequest() *bldr_web_plugin.HandleWebViewViaPluginRequest {
	return &bldr_web_plugin.HandleWebViewViaPluginRequest{
		HandlePluginId: c.GetHandlePluginId(),
		WebViewIdRe:    c.GetWebViewIdRe(),
	}
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
