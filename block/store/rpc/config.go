package block_store_rpc

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
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
	if c.GetBlockStoreId() == "" {
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
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return ot.EqualVT(c)
}

var _ config.Config = ((*Config)(nil))
