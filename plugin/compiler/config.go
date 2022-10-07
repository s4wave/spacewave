package plugin_compiler

import (
	"github.com/aperturerobotics/bldr/plugin"
	builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"golang.org/x/mod/module"
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
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if len(c.GetPlatformId()) == 0 {
		return plugin.ErrEmptyPlatformID
	}
	if len(c.GetPluginId()) == 0 {
		return plugin.ErrEmptyPluginID
	}
	for i, impPath := range c.GetGoPackages() {
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "go_packages[%d]: invalid import path", i)
		}
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// SetPluginId configures the plugin ID to build.
func (c *Config) SetPluginId(pluginID string) {
	c.PluginId = pluginID
}

// SetEngineId configures the world engine ID to attach to.
func (c *Config) SetEngineId(worldEngineID string) {
	c.EngineId = worldEngineID
}

// SetPluginHostKey configures the plugin host object key.
func (c *Config) SetPluginHostKey(pluginHostObjKey string) {
	c.PluginHostKey = pluginHostObjKey
}

// SetPlatformId configures the platform ID to compile for.
func (c *Config) SetPlatformId(platformID string) {
	c.PlatformId = platformID
}

// _ is a type assertion
var _ builder.Config = ((*Config)(nil))
