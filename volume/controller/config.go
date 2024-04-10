package volume_controller

import (
	"time"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/pkg/errors"
)

// Validate validates the config.
func (c *Config) Validate() error {
	if _, err := c.ParseBlockStoreWritebackTimeoutDur(); err != nil {
		return errors.Wrap(err, "block_store_writeback_timeout_dur")
	}
	if err := c.GetBlockStoreWritebackPutOpts().Validate(); err != nil {
		return errors.Wrap(err, "block_store_writeback_put_opts")
	}
	return nil
}

// ParseBlockStoreWritebackTimeoutDur parses the block store writeback timeout field.
// Returns 0, nil if empty.
func (c *Config) ParseBlockStoreWritebackTimeoutDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetBlockStoreWritebackTimeoutDur())
}
