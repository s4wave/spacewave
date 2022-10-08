package bldr_project_controller

import (
	"path"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewConfig constructs the configuration.
func NewConfig(
	repoRoot,
	workingPath string,
	projConfig *bldr_project.ProjectConfig,
	startProject bool,
	engineID string,
	pluginHostKey string,
	platformID string,
) *Config {
	return &Config{
		SourcePath:    repoRoot,
		WorkingPath:   workingPath,
		ProjectConfig: projConfig,
		StartProject:  startProject,
		EngineId:      engineID,
		PluginHostKey: pluginHostKey,
		PlatformId:    platformID,
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

	return proto.Equal(c, oc)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetSourcePath() == "" {
		return errors.Wrap(plugin.ErrEmptyPath, "source path")
	}
	if !path.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if c.GetWorkingPath() == "" {
		return errors.Wrap(plugin.ErrEmptyPath, "working path")
	}
	if !path.IsAbs(c.GetWorkingPath()) {
		return errors.New("working path must be absolute")
	}
	if err := c.GetProjectConfig().Validate(); err != nil {
		return errors.Wrap(err, "project_config")
	}
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if c.GetPluginHostKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "plugin host key")
	}
	if c.GetPlatformId() == "" {
		return plugin.ErrEmptyPlatformID
	}
	return nil
}

// CopyToPluginBuilder copies config fields to a plugin builder config.
func (c *Config) CopyToPluginBuilder(conf plugin_builder.Config) {
	conf.SetEngineId(c.GetEngineId())
	conf.SetPlatformId(c.GetPlatformId())
	conf.SetPluginHostKey(c.GetPluginHostKey())
	conf.SetSourcePath(c.GetSourcePath())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
