package block_store_http_lookup

import (
	"net/url"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(bucketID, uri string) *Config {
	return &Config{
		BucketId: bucketID,
		Url:      uri,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return bucket.ErrBucketIDEmpty
	}
	if _, err := c.ParseURL(); err != nil {
		return err
	}
	return nil
}

// ParseURL parses the url field or returns nil, nil if not set.
func (c *Config) ParseURL() (*url.URL, error) {
	return confparse.ParseURL(c.GetUrl())
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

var _ config.Config = ((*Config)(nil))
