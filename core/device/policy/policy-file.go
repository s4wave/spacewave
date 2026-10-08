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

// ReadFile reads the Device policy file under stateRoot, ignoring unknown JSON fields.
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
	return nil
}
