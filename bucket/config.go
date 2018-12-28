package bucket

import (
	"errors"
)

var (
	ErrIdTooShort   = errors.New("bucket id too short or empty")
	ErrVersionEmpty = errors.New("version id starts at one")
)

// NewConfig constructs a new bucket config.
func NewConfig(id string, version uint32, recConfigs []*ReconcilerConfig) (*Config, error) {
	c := &Config{Id: id, Version: version, Reconcilers: recConfigs}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate does cursory validation of the config.
func (c *Config) Validate() error {
	if len(c.GetId()) < 16 {
		return ErrIdTooShort
	}
	if c.GetVersion() == 0 {
		return ErrVersionEmpty
	}

	return nil
}
