package unixfs_linkedfolder

import (
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ConfigID is the string used to identify this config object.
const ConfigID = "unixfs/linked-folder"

// Validate requires the World engine, the Drive and an absolute folder.
func (c *Config) Validate() error {
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if c.GetObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "drive")
	}
	if !filepath.IsAbs(c.GetFolder()) {
		return errors.Errorf("folder must be an absolute path: %q", c.GetFolder())
	}
	return nil
}

// DestPath returns the parts of the path in the Drive the folder links to.
func (c *Config) DestPath() []string {
	var dest []string
	for part := range strings.SplitSeq(c.GetPath(), "/") {
		if part != "" {
			dest = append(dest, part)
		}
	}
	return dest
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
