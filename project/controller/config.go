package bldr_project_controller

import (
	"path/filepath"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewConfig constructs the configuration.
func NewConfig(
	repoRoot,
	workingPath string,
	projConfig *bldr_project.ProjectConfig,
	watch, start bool,
) *Config {
	return &Config{
		SourcePath:    repoRoot,
		WorkingPath:   workingPath,
		ProjectConfig: projConfig,
		Watch:         watch,
		Start:         start,
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
		return errors.Wrap(bldr_manifest.ErrEmptyPath, "source path")
	}
	if !filepath.IsAbs(c.GetSourcePath()) {
		return errors.New("source path must be absolute")
	}
	if c.GetWorkingPath() == "" {
		return errors.Wrap(bldr_manifest.ErrEmptyPath, "working path")
	}
	if !filepath.IsAbs(c.GetWorkingPath()) {
		return errors.New("working path must be absolute")
	}
	if err := c.GetProjectConfig().Validate(); err != nil {
		return errors.Wrap(err, "project_config")
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
