package forge_lib_docker

import (
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetImage() == "" {
		return errors.New("image cannot be empty")
	}
	if err := validateEnvMap(c.GetDockerEnv()); err != nil {
		return errors.Wrap(err, "docker_env")
	}
	if err := validateEnvMap(c.GetEnv()); err != nil {
		return errors.Wrap(err, "env")
	}
	for _, mount := range c.GetMounts() {
		if mount.GetHostPath() == "" {
			return errors.New("mount host_path cannot be empty")
		}
		if mount.GetContainerPath() == "" {
			return errors.New("mount container_path cannot be empty")
		}
		if strings.Contains(mount.GetHostPath(), ",") || strings.Contains(mount.GetContainerPath(), ",") {
			return errors.New("mount paths cannot contain comma")
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

// MarshalBlock marshals the block to binary.
func (c *Config) MarshalBlock() ([]byte, error) {
	return c.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (c *Config) UnmarshalBlock(data []byte) error {
	return c.UnmarshalVT(data)
}

// validateEnvMap rejects empty keys and keys containing '='.
func validateEnvMap(env map[string]string) error {
	for key := range env {
		if key == "" {
			return errors.New("key cannot be empty")
		}
		if strings.Contains(key, "=") {
			return errors.Errorf("key cannot contain =: %s", key)
		}
	}
	return nil
}

// sortedMapKeys returns the map keys in sorted order.
func sortedMapKeys(vals map[string]string) []string {
	keys := make([]string, 0, len(vals))
	for key := range vals {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// _ is a type assertion
var (
	_ config.Config = (*Config)(nil)
	_ block.Block   = (*Config)(nil)
)
