package bucket

import (
	"strings"
)

// NewConfig constructs a new bucket config.
func NewConfig(
	id string,
	rev uint32,
	lkConfig *LookupConfig,
) (*Config, error) {
	c := &Config{
		Id:     id,
		Rev:    rev,
		Lookup: lkConfig,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate does cursory validation of the config.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.GetId()) == "" {
		return ErrBucketIDEmpty
	}
	if c.GetRev() == 0 {
		return ErrRevEmpty
	}

	return nil
}
