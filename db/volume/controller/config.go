package volume_controller

import (
	"time"

	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/pkg/errors"
)

// defaultGCInterval is the default GC sweep interval.
const defaultGCInterval = time.Minute

// Validate validates the config.
func (c *Config) Validate() error {
	if _, err := c.ParseBlockStoreWritebackTimeoutDur(); err != nil {
		return errors.Wrap(err, "block_store_writeback_timeout_dur")
	}
	if err := c.GetBlockStoreWritebackPutOpts().Validate(); err != nil {
		return errors.Wrap(err, "block_store_writeback_put_opts")
	}
	if _, err := c.ParseGCIntervalDur(); err != nil {
		return errors.Wrap(err, "gc_interval_dur")
	}
	return nil
}

// ParseBlockStoreWritebackTimeoutDur parses the block store writeback timeout field.
// Returns 0, nil if empty.
func (c *Config) ParseBlockStoreWritebackTimeoutDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetBlockStoreWritebackTimeoutDur())
}

// ParseGCIntervalDur parses the GC interval duration field.
// Returns defaultGCInterval if empty.
func (c *Config) ParseGCIntervalDur() (time.Duration, error) {
	dur, err := confparse.ParseDuration(c.GetGcIntervalDur())
	if err != nil {
		return 0, err
	}
	if dur == 0 && c.GetGcIntervalDur() == "" {
		return defaultGCInterval, nil
	}
	return dur, nil
}
