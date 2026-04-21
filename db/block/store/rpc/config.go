package block_store_rpc

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId, serviceID string, readOnly bool, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		ServiceId:    serviceID,
		ReadOnly:     readOnly,
		BucketIds:    bucketIDs,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" && len(c.GetBlockStoreIds()) == 0 {
		return block_store.ErrBlockStoreIDEmpty
	}
	if c.GetServiceId() == "" {
		return srpc.ErrEmptyServiceID
	}
	if err := c.GetForceHashType().Validate(); err != nil {
		return errors.Wrap(err, "force_hash_type")
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
