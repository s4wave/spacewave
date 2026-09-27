package plugin_space_runtime

import (
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetSpace() == nil {
		return errors.New("space config must be specified")
	}
	return c.GetSpace().Validate()
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	peer, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.GetSpace().EqualVT(peer.GetSpace()) && slices.Equal(
		canonicalAppPluginIDs(c.GetAppPluginIds()), canonicalAppPluginIDs(peer.GetAppPluginIds()),
	)
}

// canonicalAppPluginIDs owns a sorted, duplicate-free application declaration.
func canonicalAppPluginIDs(ids []string) []string {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) != 0 && ids[0] == "" {
		ids = ids[1:]
	}
	return ids
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
