package resource_listener

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}

// DetermineSocketPath returns the configured socket path. An explicit path
// takes precedence; otherwise a project state path, when present, scopes the
// socket before the shared storage root fallback.
func (c *Config) DetermineSocketPath() (string, error) {
	if socketPath := c.GetListenerSocketPath(); socketPath != "" {
		return socketPath, nil
	}
	projectID := c.GetStorageProjectId()
	if projectID == "" {
		return "", nil
	}
	if socketPath := os.Getenv(storagepath.SocketPathEnvVar(projectID)); socketPath != "" {
		return socketPath, nil
	}
	if statePath := os.Getenv(storagepath.StatePathEnvVar(projectID)); statePath != "" {
		return filepath.Join(statePath, projectID+".sock"), nil
	}
	storageRoot, err := storagepath.DetermineStorageRoot(projectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(storageRoot, projectID+".sock"), nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
