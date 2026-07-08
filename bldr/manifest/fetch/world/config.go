package manifest_fetch_world

import (
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	manifest_fetch "github.com/s4wave/spacewave/bldr/manifest/fetch"
	"github.com/s4wave/spacewave/db/world"
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
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if _, err := c.ParseFetchManifestIdRe(); err != nil {
		return err
	}
	if _, err := c.ParsePointerTTLDur(); err != nil {
		return errors.Wrap(err, "pointer_ttl_dur")
	}
	if c.GetCdnSpaceId() != "" && c.GetCdnBaseUrl() == "" {
		return errors.New("cdn_base_url cannot be empty when cdn_space_id is set")
	}
	if c.GetCdnBaseUrl() != "" && c.GetCdnSpaceId() == "" {
		return errors.New("cdn_space_id cannot be empty when cdn_base_url is set")
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

// ParsePointerTTLDur parses the root pointer TTL field.
func (c *Config) ParsePointerTTLDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetPointerTtlDur())
}

// _ is a type assertion
var _ manifest_fetch.Config = ((*Config)(nil))
