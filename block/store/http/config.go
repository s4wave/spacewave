package block_store_http

import (
	"net/url"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId, url string, readOnly bool, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		Url:          url,
		ReadOnly:     readOnly,
		BucketIds:    bucketIDs,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	u, err := c.ParseURL()
	if err != nil {
		return err
	}
	if u == nil {
		return errors.New("url cannot be empty")
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
