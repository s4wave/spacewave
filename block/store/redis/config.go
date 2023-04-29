package block_store_redis

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/aperturerobotics/hydra/block/store"
	store_kvtx_redis "github.com/aperturerobotics/hydra/store/kvtx/redis"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId string, clientConfig *store_kvtx_redis.ClientConfig) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		Client:       clientConfig,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if err := c.GetClient().Validate(); err != nil {
		return errors.Wrap(err, "client")
	}
	return nil
}

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

	return ot.EqualVT(c)
}

var _ config.Config = ((*Config)(nil))
