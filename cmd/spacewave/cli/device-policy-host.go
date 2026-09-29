//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// devicePolicyHostSource forwards daemon policy snapshots to the plugin host.
type devicePolicyHostSource struct {
	// store owns the policy value and its revision watch.
	store *device_policy.PolicyStore
	// statePath locates the enrolled Device identity for the Worker claim.
	statePath string
}

// WaitDevicePolicy waits for the first or next policy revision and attaches the
// enrolled Device identity from the daemon's local setup record.
func (s *devicePolicyHostSource) WaitDevicePolicy(ctx context.Context, last []byte) ([]byte, string, uint64, error) {
	// Decode the caller's last-seen policy as the wait baseline.
	var previous *device_policy.DevicePolicy
	if len(last) != 0 {
		previous = &device_policy.DevicePolicy{}
		if err := previous.UnmarshalVT(last); err != nil {
			return nil, "", 0, errors.Wrap(err, "decode previous policy")
		}
	}

	// Wait for a new revision and encode the policy.
	policy, err := s.store.WaitChange(ctx, previous)
	if err != nil {
		return nil, "", 0, err
	}
	data, err := policy.MarshalVT()
	if err != nil {
		return nil, "", 0, errors.Wrap(err, "encode device policy")
	}

	// Attach the enrolled Device identity when a projection record exists.
	record, ok, err := deviceLauncherProjectionTarget(s.statePath)
	if err != nil {
		return nil, "", 0, err
	}
	if !ok {
		return data, "", policy.GetRevision(), nil
	}
	return data, record.DeviceObjectKey, policy.GetRevision(), nil
}
