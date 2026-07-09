package resource_listener

import (
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
// takes precedence; otherwise StorageProjectId uses the same project storage
// root as the rest of the native runtime.
func (c *Config) DetermineSocketPath() (string, error) {
	if socketPath := c.GetListenerSocketPath(); socketPath != "" {
		return socketPath, nil
	}
	projectID := c.GetStorageProjectId()
	if projectID == "" {
		return "", nil
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
var _ config.Config = ((*Config)(nil))
