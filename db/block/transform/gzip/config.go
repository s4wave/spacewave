package transform_gzip

import (
	"compress/gzip"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// Validate validates the configuration.
func (c *Config) Validate() error {
	level := c.GetCompressionLevel()
	if level == 0 {
		return nil
	}
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		return errors.Errorf("gzip: invalid compression level: %d", level)
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	_, ok := other.(*Config)
	return ok
}

// EffectiveCompressionLevel returns the stdlib gzip compression level.
func (c *Config) EffectiveCompressionLevel() int {
	level := c.GetCompressionLevel()
	if level == 0 {
		return gzip.DefaultCompression
	}
	return int(level)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
