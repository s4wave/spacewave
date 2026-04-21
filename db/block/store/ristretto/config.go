package block_store_ristretto

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/s4wave/spacewave/db/block/store"
	kvtx_ristretto "github.com/s4wave/spacewave/db/store/kvtx/ristretto"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId string, ristrettoConfig *kvtx_ristretto.Config, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		Ristretto:    ristrettoConfig,
		BucketIds:    bucketIDs,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if err := c.GetRistretto().Validate(); err != nil {
		return errors.Wrap(err, "ristretto")
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
