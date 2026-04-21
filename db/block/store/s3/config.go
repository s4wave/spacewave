package block_store_s3

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId string, clientConfig *ClientConfig, bucketName, objectPrefix string, readOnly bool, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId: blockStoreId,
		Client:       clientConfig,
		BucketName:   bucketName,
		ObjectPrefix: objectPrefix,
		ReadOnly:     readOnly,
		BucketIds:    bucketIDs,
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
	return config.EqualsConfig[*Config](c, other)
}

var _ config.Config = ((*Config)(nil))
