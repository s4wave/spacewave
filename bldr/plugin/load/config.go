package bldr_plugin_load

import (
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate checks the config.
func (c *Config) Validate() error {
	ids := c.CleanupPluginIds()
	if len(ids) == 0 {
		return bldr_plugin.ErrEmptyPluginID
	}
	for _, id := range ids {
		if err := bldr_plugin.ValidatePluginID(id, false); err != nil {
			return err
		}
	}
	return nil
}

// CleanupPluginIds returns a sorted copy of the list of plugin IDs to load.
func (c *Config) CleanupPluginIds() []string {
	ids := append([]string{c.GetPluginId()}, c.GetPluginIds()...)
	for i := range ids {
		ids[i] = strings.TrimSpace(ids[i])
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids[0] == "" {
		ids = ids[1:]
	}
	return ids
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
