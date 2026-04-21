package transform_lz4

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pierrec/lz4/v4"
	"github.com/pkg/errors"
)

// ToCompressionLevel converts the level into a compression level.
func ToCompressionLevel(level uint32) (lz4.CompressionLevel, error) {
	if level == 0 {
		return lz4.Fast, nil
	}
	if level > 9 {
		return 0, errors.Errorf("%v: %v", lz4.ErrOptionInvalidCompressionLevel, level)
	}
	return lz4.CompressionLevel(1 << (8 + level)), nil
}

// DefaultBlockSize matches the default block size in the lz4 library.
const DefaultBlockSize = lz4.Block4Mb

// ToBlockSize converts the size into a lz4 block size.
func (b BlockSize) ToBlockSize() (lz4.BlockSize, error) {
	if b == 0 {
		return DefaultBlockSize, nil
	}
	switch b {
	case 0:
		return DefaultBlockSize, nil
	case 1:
		return lz4.Block64Kb, nil
	case 2:
		return lz4.Block256Kb, nil
	case 3:
		return lz4.Block1Mb, nil
	default:
		return 0, errors.Errorf("%v: %v", lz4.ErrOptionInvalidBlockSize, b)
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if _, err := c.GetBlockSize().ToBlockSize(); err != nil {
		return err
	}
	if _, err := ToCompressionLevel(c.GetCompressionLevel()); err != nil {
		return err
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

// ToOptions converts the config into lz4 options.
func (c *Config) ToOptions() []lz4.Option {
	var opts []lz4.Option
	if bs, err := c.GetBlockSize().ToBlockSize(); err == nil && bs != DefaultBlockSize {
		opts = append(opts, lz4.BlockSizeOption(bs))
	}
	if c.GetBlockChecksum() {
		opts = append(opts, lz4.BlockChecksumOption(true))
	}
	if c.GetDisableChecksum() {
		opts = append(opts, lz4.ChecksumOption(false))
	}
	if cl, err := ToCompressionLevel(c.GetCompressionLevel()); cl != 0 && err == nil {
		opts = append(opts, lz4.CompressionLevelOption(cl))
	}
	return opts
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
