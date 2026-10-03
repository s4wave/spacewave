package bldr_web_pkg_compiler

import (
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/web/pkg/compiler"

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
	// Validate the optional project that supplies the compiler inputs.
	if projID := c.GetProjectId(); projID != "" {
		if err := bldr_project.ValidateProjectID(projID); err != nil {
			return errors.Wrap(err, "project_id")
		}
	}

	// Validate the web plugin that serves package lookups.
	if err := bldr_plugin.ValidatePluginID(c.GetWebPluginId(), false); err != nil {
		return err
	}

	// Validate the controllers included in the compiled plugin.
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}

	// Validate the controllers included in the plugin host.
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
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

// _ is a type assertion
var _ builder.ControllerConfig = (*Config)(nil)
