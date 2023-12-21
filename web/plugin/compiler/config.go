package bldr_web_plugin_compiler

import (
	"strings"

	builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_project "github.com/aperturerobotics/bldr/project"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
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
	if projID := c.GetProjectId(); projID != "" {
		if err := bldr_project.ValidateProjectID(projID); err != nil {
			return errors.Wrap(err, "project_id")
		}
	}
	if err := configset_proto.ConfigSetMap(c.GetConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "config_set")
	}
	if err := configset_proto.ConfigSetMap(c.GetHostConfigSet()).Validate(); err != nil {
		return errors.Wrap(err, "host_config_set")
	}
	if electronPkg := c.GetElectronPkg(); electronPkg != "" {
		// split on version
		chk := strings.TrimSpace(electronPkg)
		verIdx := strings.LastIndex(electronPkg, "@")
		if verIdx != -1 && verIdx > 0 {
			chk = electronPkg[:verIdx]
		}
		if err := web_pkg.ValidateWebPkgId(chk); err != nil {
			return errors.Errorf("electron_pkg: invalid web pkg id: %s", chk)
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

// _ is a type assertion
var _ builder.ControllerConfig = ((*Config)(nil))
