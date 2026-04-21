package block_store_inmem

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId string, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		BucketIds:    bucketIDs,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	return nil
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
