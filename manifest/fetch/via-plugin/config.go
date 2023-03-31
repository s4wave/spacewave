package manifest_fetch_viaplugin

import (
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
	manifest_fetch "github.com/aperturerobotics/bldr/manifest/fetch"
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
	if _, err := c.ParseFetchManifestIdRegex(); err != nil {
		return err
	}
	return nil
}

// SetFetchManifestIdRegex sets the fetch_manifest_id regex.
func (c *Config) SetFetchManifestIdRegex(re string) {
	c.FetchManifestIdRegex = re
}

// ParseFetchManifestIdRegex parses the fetch_manifest_id regex.
// Returns nil if the field was empty.
func (c *Config) ParseFetchManifestIdRegex() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetFetchManifestIdRegex())
}

// _ is a type assertion
var _ manifest_fetch.Config = ((*Config)(nil))
