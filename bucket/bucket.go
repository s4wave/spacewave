package bucket

import (
	"errors"
)

var (
	ErrIdTooShort   = errors.New("bucket id too short or empty")
	ErrVersionEmpty = errors.New("version id starts at one")
)

// Validate does cursory validation of the config.
func (c *Config) Validate() error {
	if len(c.GetId()) < 30 {
		return ErrIdTooShort
	}
	if c.GetVersion() == 0 {
		return ErrVersionEmpty
	}

	return nil
}
