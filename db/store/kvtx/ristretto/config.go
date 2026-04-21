package store_kvtx_ristretto

import (
	"time"

	"github.com/s4wave/spacewave/net/util/confparse"
)

// Validate checks the config.
func (c *Config) Validate() error {
	if _, err := c.ParseTtlDur(); err != nil {
		return err
	}
	return nil
}

// ParseTtlDur parses the time to live duration.
func (c *Config) ParseTtlDur() (time.Duration, error) {
	return confparse.ParseDuration(c.GetTtlDur())
}
