package spacewave_loader_controller

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// defaultProjectID is the fallback project id when Config.ProjectId is empty.
const defaultProjectID = "spacewave"

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

// Validate checks the config. Both fields are optional with sensible
// defaults, so Validate always passes.
func (c *Config) Validate() error {
	return nil
}

// ResolvedProjectID returns ProjectId or the default.
func (c *Config) ResolvedProjectID() string {
	if id := c.GetProjectId(); id != "" {
		return id
	}
	return defaultProjectID
}

// defaultWatchPluginIDs is the stock plugin set the loader shows progress for
// when Config.WatchPluginIds is empty. Mirrors spacewave-dist.loadPlugins
// minus the launcher/loader pair that boots before the helper is visible.
var defaultWatchPluginIDs = []string{
	"spacewave-core",
	"spacewave-web",
	"spacewave-app",
	"web",
}

// ResolvedWatchPluginIDs returns WatchPluginIds or the default set.
func (c *Config) ResolvedWatchPluginIDs() []string {
	if ids := c.GetWatchPluginIds(); len(ids) != 0 {
		out := make([]string, len(ids))
		copy(out, ids)
		return out
	}
	out := make([]string, len(defaultWatchPluginIDs))
	copy(out, defaultWatchPluginIDs)
	return out
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
