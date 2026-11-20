package forge_lib_skiffos_audit

import (
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if !strings.HasPrefix(c.GetUpstreamRef(), "refs/") {
		return errors.Errorf("upstream_ref must be a full ref name: %q", c.GetUpstreamRef())
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
