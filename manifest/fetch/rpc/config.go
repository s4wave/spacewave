package manifest_fetch_rpc

import (
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
	manifest_fetch "github.com/aperturerobotics/bldr/manifest/fetch"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
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
	if c.GetServiceId() == "" {
		return srpc.ErrEmptyServiceID
	}
	if _, err := c.ParseFetchManifestIdRe(); err != nil {
		return err
	}
	return nil
}

// SetFetchManifestIdRe sets the fetch_manifest_id regex.
func (c *Config) SetFetchManifestIdRe(re string) {
	c.FetchManifestIdRe = re
}

// ParseFetchManifestIdRe parses the fetch_manifest_id regex.
// Returns nil if the field was empty.
func (c *Config) ParseFetchManifestIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetFetchManifestIdRe())
}

// _ is a type assertion
var _ manifest_fetch.Config = ((*Config)(nil))
