package cdn_bstore_controller

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreID, spaceID, cdnBaseURL string) *Config {
	return &Config{
		BlockStoreId: blockStoreID,
		SpaceId:      spaceID,
		CdnBaseUrl:   cdnBaseURL,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if c.GetSpaceId() == "" {
		return errors.New("space_id cannot be empty")
	}
	if c.GetCdnBaseUrl() == "" {
		return errors.New("cdn_base_url cannot be empty")
	}
	if _, err := c.ParsePointerTTLDur(); err != nil {
		return errors.Wrap(err, "pointer_ttl_dur")
	}
	if c.GetWritebackWindowBytes() < 0 {
		return errors.New("writeback_window_bytes cannot be negative")
	}
	if c.GetRangeCacheMaxBytes() < 0 {
		return errors.New("range_cache_max_bytes cannot be negative")
	}
	return nil
}

// ParsePointerTTLDur parses the root pointer TTL field.
func (c *Config) ParsePointerTTLDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetPointerTtlDur())
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
