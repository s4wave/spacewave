package device_policy

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
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
	data, err := os.ReadFile(FilePath(stateRoot))
	if os.IsNotExist(err) {
		return &DevicePolicy{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read device policy")
	}
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
	if policy == nil {
		policy = &DevicePolicy{}
	}
	if err := Validate(policy); err != nil {
		return err
	}
	data, err := policy.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "marshal device policy")
	}
	data = append(data, '\n')
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
	if policy == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(policy.GetCheckoutRoot()))
	for _, root := range policy.GetCheckoutRoot() {
		if root == nil {
			continue
		}
		name := strings.TrimSpace(root.GetName())
		if name == "" {
			return errors.New("device policy checkout-root name is required")
		}
		if _, ok := seen[name]; ok {
			return errors.Errorf("duplicate device policy checkout-root %q", name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(root.GetLocalPath()) == "" {
			return errors.Errorf("device policy checkout-root %q local path is required", name)
		}
		switch root.GetAccess() {
		case s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
			s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE:
		default:
			return errors.Errorf("device policy checkout-root %q access is required", name)
		}
	}
	seenEndpoints := make(map[string]struct{}, len(policy.GetSensorEndpoint()))
	for _, endpoint := range policy.GetSensorEndpoint() {
		if endpoint == nil {
			continue
		}
		id := strings.TrimSpace(endpoint.GetId())
		if id == "" {
			return errors.New("device policy sensor-endpoint id is required")
		}
		if _, ok := seenEndpoints[id]; ok {
			return errors.Errorf("duplicate device policy sensor-endpoint %q", id)
		}
		seenEndpoints[id] = struct{}{}
		if strings.TrimSpace(endpoint.GetEndpoint()) == "" {
			return errors.Errorf("device policy sensor-endpoint %q endpoint is required", id)
		}
		if !endpoint.GetEnabled() {
			continue
		}
		switch endpoint.GetAdapterKind() {
		case s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME:
		default:
			return errors.Errorf("device policy sensor-endpoint %q adapter kind is required", id)
		}
	}
	return nil
}
