package unixfs_access_http

import (
	"regexp"

	"github.com/aperturerobotics/controllerbus/config"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig() *Config {
	return &Config{}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetUnixfsId() == "" {
		return unixfs_errors.ErrEmptyUnixFsId
	}
	if _, err := c.ParsePathRe(); err != nil {
		return err
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// ParsePathRe parses the path regex field.
// Returns nil if the field was empty.
func (c *Config) ParsePathRe() (*regexp.Regexp, error) {
	r := c.GetPathRe()
	if r == "" {
		return nil, nil
	}
	return regexp.Compile(r)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
