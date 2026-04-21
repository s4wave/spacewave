package block_store_overlay

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreId, lowerBlockStoreID, upperBlockStoreID string, overlayMode block.OverlayMode, bucketIDs []string) *Config {
	return &Config{
		BlockStoreId:      blockStoreId,
		LowerBlockStoreId: lowerBlockStoreID,
		UpperBlockStoreId: upperBlockStoreID,
		OverlayMode:       overlayMode,
		BucketIds:         bucketIDs,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if c.GetLowerBlockStoreId() == "" {
		return errors.Wrap(block_store.ErrBlockStoreIDEmpty, "lower_block_store")
	}
	if c.GetUpperBlockStoreId() == "" {
		return errors.Wrap(block_store.ErrBlockStoreIDEmpty, "upper_block_store")
	}
	if c.GetLowerBlockStoreId() == c.GetUpperBlockStoreId() {
		return errors.New("lower and upper block store cannot be the same")
	}
	if c.GetBlockStoreId() == c.GetLowerBlockStoreId() {
		return errors.New("block store id and lower block store id cannot be the same")
	}
	if c.GetBlockStoreId() == c.GetUpperBlockStoreId() {
		return errors.New("block store id and upper block store id cannot be the same")
	}
	if _, err := c.ParseWritebackTimeoutDur(); err != nil {
		return errors.Wrap(err, "writeback_timeout_dur")
	}
	if err := c.GetWritebackPutOpts().Validate(); err != nil {
		return errors.Wrap(err, "writeback_put_opts")
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

// ParseWritebackTimeoutDur parses the block store writeback timeout field.
// Returns 0, nil if empty.
func (c *Config) ParseWritebackTimeoutDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetWritebackTimeoutDur())
}

var _ config.Config = ((*Config)(nil))
