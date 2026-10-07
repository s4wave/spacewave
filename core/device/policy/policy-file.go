package device_policy

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	// StateDir is the daemon state-root directory containing Device-local files.
	StateDir = "device"
	// StateFile is the generated-JSON policy file name under StateDir.
	StateFile = "policy.json"
)

// FilePath returns the Device policy file path under stateRoot.
func FilePath(stateRoot string) string {
	return filepath.Join(stateRoot, StateDir, StateFile)
}

// ReadFile reads the Device policy file under stateRoot.
func ReadFile(stateRoot string) (*DevicePolicy, error) {
	// Read the policy file, treating a missing file as an empty policy.
	data, err := os.ReadFile(FilePath(stateRoot))
	if os.IsNotExist(err) {
		return &DevicePolicy{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read device policy")
	}

	// Parse and validate the persisted Device policy.
	policy := &DevicePolicy{}
	if err := policy.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "parse device policy")
	}
	if err := Validate(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// WriteFile writes the Device policy file under stateRoot.
func WriteFile(stateRoot string, policy *DevicePolicy) error {
	// Substitute an empty policy for nil and validate the result.
	if policy == nil {
		policy = &DevicePolicy{}
	}
	if err := Validate(policy); err != nil {
		return err
	}

	// Marshal the policy with a trailing newline.
	data, err := policy.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "marshal device policy")
	}
	data = append(data, '\n')

	// Write the policy file into its state directory.
	path := FilePath(stateRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "create device policy state directory")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errors.Wrap(err, "write device policy")
	}
	return nil
}

// Validate checks the persisted Device policy shape.
func Validate(policy *DevicePolicy) error {
	// Treat a nil policy as valid.
	if policy == nil {
		return nil
	}

	// Reject node type IDs that are missing or duplicated.
	seenTypes := make(map[string]struct{}, len(policy.GetNodeTypeId()))
	for _, typeID := range policy.GetNodeTypeId() {
		if strings.TrimSpace(typeID) == "" {
			return errors.New("device policy node type id is required")
		}
		if _, ok := seenTypes[typeID]; ok {
			return errors.Errorf("duplicate device policy node type %q", typeID)
		}
		seenTypes[typeID] = struct{}{}
	}

	// Require the forge-worker's object key and resource limits.
	if fw := policy.GetForgeWorker(); fw != nil {
		if strings.TrimSpace(fw.GetWorkerObjectKey()) == "" {
			return errors.New("device policy forge-worker worker object key is required")
		}
		if fw.GetMilliCpu() == 0 {
			return errors.New("device policy forge-worker milli_cpu must be set")
		}
		if fw.GetMemoryBytes() == 0 {
			return errors.New("device policy forge-worker memory_bytes must be set")
		}
		if len(fw.GetBackends()) == 0 {
			return errors.New("device policy forge-worker backends must not be empty")
		}
		// Reject empty, whitespace-containing, or duplicate backends.
		seenBackends := make(map[string]struct{}, len(fw.GetBackends()))
		for _, backend := range fw.GetBackends() {
			if strings.TrimSpace(backend) == "" {
				return errors.New("device policy forge-worker backend must not be empty")
			}
			if strings.ContainsAny(backend, ", \t\r\n") {
				return errors.Errorf("device policy forge-worker backend %q cannot contain comma or whitespace", backend)
			}
			if _, ok := seenBackends[backend]; ok {
				return errors.Errorf("duplicate device policy forge-worker backend %q", backend)
			}
			seenBackends[backend] = struct{}{}
		}
	}
	return nil
}
