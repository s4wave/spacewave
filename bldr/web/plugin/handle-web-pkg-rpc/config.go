package bldr_web_plugin_handle_web_pkg_rpc

import (
	"regexp"

	"github.com/s4wave/spacewave/net/util/confparse"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
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
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "web_plugin_id")
	}
	if c.GetHandlePluginId() == "" {
		return errors.Wrap(bldr_plugin.ErrEmptyPluginID, "handle_plugin_id")
	}
	if _, err := c.ParseWebPkgIdRe(); err != nil {
		return err
	}
	return nil
}

// ParseWebPkgIdRe parses the handle web view id regex.
// Returns nil if the field was empty.
func (c *Config) ParseWebPkgIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetWebPkgIdRe())
}

// ToRequest converts the config into a request.
func (c *Config) ToRequest() *bldr_web_plugin.HandleWebPkgViaPluginRequest {
	return &bldr_web_plugin.HandleWebPkgViaPluginRequest{
		HandlePluginId:   c.GetHandlePluginId(),
		WebPkgIdRe:       c.GetWebPkgIdRe(),
		WebPkgIdPrefixes: c.GetWebPkgIdPrefixes(),
		WebPkgIdList:     c.GetWebPkgIdList(),
	}
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
