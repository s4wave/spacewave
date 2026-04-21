package block_store_rpc_lookup

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/aperturerobotics/starpc/srpc"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(bucketID, serviceID string) *Config {
	return &Config{
		BucketId:  bucketID,
		ServiceId: serviceID,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return bucket.ErrBucketIDEmpty
	}
	if c.GetServiceId() == "" {
		return srpc.ErrEmptyServiceID
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
