package reconciler_example

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_store "github.com/aperturerobotics/hydra/block/store"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/reconciler"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return bucket.ErrBucketIDEmpty
	}
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if c.GetReconcilerId() == "" {
		return reconciler.ErrReconcilerIDEmpty
	}
	return nil
}

// SetBucketId sets the bucket ID field.
func (c *Config) SetBucketId(id string) {
	c.BucketId = id
}

// SetBlockStoreId sets the blockStore ID field.
func (c *Config) SetBlockStoreId(id string) {
	c.BlockStoreId = id
}

// SetReconcilerId sets the reconciler ID field.
func (c *Config) SetReconcilerId(id string) {
	c.ReconcilerId = id
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ reconciler.Config = ((*Config)(nil))
