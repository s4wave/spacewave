package bldr_plugin_builder_controller

import (
	builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	backoff "github.com/aperturerobotics/util/backoff"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewConfig constructs the configuration.
func NewConfig(
	builderConfig *builder.PluginBuilderConfig,
	builderControllerConfig *configset_proto.ControllerConfig,
	buildBackoff *backoff.Backoff,
) *Config {
	return &Config{
		BuilderConfig:           builderConfig,
		BuilderControllerConfig: builderControllerConfig,
		BuildBackoff:            buildBackoff,
	}
}

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	oc, ok := c2.(*Config)
	if !ok {
		return false
	}

	return c.EqualVT(oc)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.GetBuilderControllerConfig().Validate(); err != nil {
		return err
	}
	if err := c.GetBuilderConfig().Validate(); err != nil {
		return err
	}
	if err := c.GetBuildBackoff().Validate(true); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
