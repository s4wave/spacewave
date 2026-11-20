package forge_lib_skiffos

import (
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	// The checkout must be absolute, since the build runs from it.
	if !filepath.IsAbs(c.GetSkiffDir()) {
		return errors.Errorf("skiff_dir must be an absolute path: %q", c.GetSkiffDir())
	}

	// The workspace names one directory under workspaces/.
	ws := c.GetWorkspace()
	if ws == "" || strings.ContainsAny(ws, "/\\") || strings.HasPrefix(ws, ".") {
		return errors.Errorf("workspace must be a plain name: %q", ws)
	}

	// SKIFF_CONFIG joins the configs with commas, so none may hold one.
	if len(c.GetConfigs()) == 0 {
		return errors.New("configs: at least one SkiffOS config is required")
	}
	for i, conf := range c.GetConfigs() {
		if conf == "" || strings.Contains(conf, ",") {
			return errors.Errorf("configs[%d]: invalid config %q", i, conf)
		}
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*Config)
	return ok && c.EqualVT(oc)
}

// _ is a type assertion
var (
	_ config.Config = (*Config)(nil)
	_ block.Block   = (*Config)(nil)
)
