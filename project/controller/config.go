package bldr_project_controller

import (
	"path"

	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewConfig constructs the configuration.
func NewConfig(repoRoot string, projConfig *bldr_project.ProjectConfig, startProject bool) *Config {
	return &Config{
		SourcePath:    repoRoot,
		ProjectConfig: projConfig,
		StartProject:  startProject,
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
		return errors.New("source path must be specified")
	}
	if !path.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if err := c.GetProjectConfig().Validate(); err != nil {
		return errors.Wrap(err, "project_config")
	}
	return nil
}
