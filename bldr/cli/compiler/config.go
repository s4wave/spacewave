package bldr_cli_compiler

import (
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"golang.org/x/mod/module"
)

// ConfigID is the config identifier.
const ConfigID = "bldr/cli/compiler"

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
	return ot.EqualVT(c)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if projID := c.GetProjectId(); projID != "" {
		if err := bldr_project.ValidateProjectID(projID); err != nil {
			return errors.Wrap(err, "project_id")
		}
	}
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	for i, impPath := range c.GetGoPkgs() {
		impPath = strings.TrimPrefix(impPath, "./")
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "go_pkgs[%d]: invalid import path", i)
		}
	}
	for i, impPath := range c.GetCliPkgs() {
		impPath = strings.TrimPrefix(impPath, "./")
		if err := module.CheckImportPath(impPath); err != nil {
			return errors.Wrapf(err, "cli_pkgs[%d]: invalid import path", i)
		}
	}
	return nil
}

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
