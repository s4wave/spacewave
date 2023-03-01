package bldr_project_controller

import (
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
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
	peerID string,
	pluginHostKey string,
	pluginPlatformID string,
	buildType string,
	disableWatch bool,
) *Config {
	return &Config{
		SourcePath:       repoRoot,
		WorkingPath:      workingPath,
		ProjectConfig:    projConfig,
		StartProject:     startProject,
		EngineId:         engineID,
		PeerId:           peerID,
		PluginHostKey:    pluginHostKey,
		PluginPlatformId: pluginPlatformID,
		BuildType:        buildType,
		DisableWatch:     disableWatch,
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
	if c.GetPluginPlatformId() == "" {
		return plugin.ErrEmptyPlatformID
	}
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if err := plugin.ToBuildType(c.GetBuildType()).Validate(false); err != nil {
		return err
	}
	return nil
}

// ToPluginBuilderConfig converts config fields to a plugin builder config.
func (c *Config) ToPluginBuilderConfig(pluginID, workingPath string) *plugin_builder.PluginBuilderConfig {
	return &plugin_builder.PluginBuilderConfig{
		EngineId:         c.GetEngineId(),
		PeerId:           c.GetPeerId(),
		PluginPlatformId: c.GetPluginPlatformId(),
		PluginHostKey:    c.GetPluginHostKey(),
		SourcePath:       c.GetSourcePath(),
		BuildType:        c.GetBuildType(),
		PluginId:         pluginID,
		WorkingPath:      workingPath,
	}
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
