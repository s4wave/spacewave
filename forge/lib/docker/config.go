package forge_lib_docker

import (
	"path"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate the container image, resource requests, environments, and mounts.
	if c.GetImage() == "" {
		return errors.New("image cannot be empty")
	}
	if c.GetMilliCpu() == 0 || c.GetMemoryBytes() == 0 {
		return errors.New("docker milli_cpu and memory_bytes requests must be set")
	}
	if err := c.GetCards().Validate(); err != nil {
		return errors.Wrap(err, "cards")
	}
	for _, device := range c.GetDevices() {
		if host, _, _ := strings.Cut(device, ":"); isCardDevice(host) {
			return errors.Errorf("devices: %s is a graphics card node; request cards instead", host)
		}
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

	// Each input names a checkout directory and an absolute container path.
	for name, path := range c.GetInputMounts() {
		if err := forge_lib_util_inputfs.ValidateName(name); err != nil {
			return errors.Wrap(err, "input_mounts")
		}
		if !strings.HasPrefix(path, "/") || strings.Contains(path, ",") {
			return errors.Errorf("input_mounts[%s]: container path must be absolute without comma: %q", name, path)
		}
	}

	// Declared outputs need a directory to land in.
	return validateOutputs(c.GetOutputDir(), c.GetOutputs())
}

// isCardDevice reports whether a host device path is a graphics card node:
// under /dev/dri, the ROCm compute interface /dev/kfd, or an NVIDIA node. A
// container reaches a card only through a card request, so admission has
// reserved every card it uses.
func isCardDevice(host string) bool {
	host = path.Clean(host)
	return host == "/dev/kfd" || host == "/dev/dri" || strings.HasPrefix(host, "/dev/dri/") || strings.HasPrefix(host, "/dev/nvidia")
}

// validateOutputs requires an output directory for declared outputs, each a
// distinct plain file name within it.
func validateOutputs(dir string, outputs []string) error {
	// Require a mountable directory whenever outputs are declared.
	if len(outputs) == 0 {
		return nil
	}
	if dir == "" {
		return errors.New("output_dir must be set with outputs")
	}
	if strings.Contains(dir, ",") {
		return errors.New("output_dir cannot contain comma")
	}

	// Require each output to name one distinct file in the directory.
	seen := make(map[string]struct{}, len(outputs))
	for _, name := range outputs {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			return errors.Errorf("output name %q must be a plain file name", name)
		}
		if _, ok := seen[name]; ok {
			return errors.Errorf("output name %q is duplicated", name)
		}
		seen[name] = struct{}{}
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
