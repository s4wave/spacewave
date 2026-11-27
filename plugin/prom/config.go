// Package prom_plugin scrapes Prometheus exporters on approved Devices and
// serves World-backed metrics through plugin RPC handlers.
// The sample store and committed query implementation live in storage.
package prom_plugin

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// DefaultCommitWindow is the commit window of a config that sets none.
const DefaultCommitWindow = time.Minute

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// CommitWindow returns the time between a Device's commits in one Space.
func (c *Config) CommitWindow() time.Duration {
	if ms := c.GetCommitWindowMs(); ms != 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return DefaultCommitWindow
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
