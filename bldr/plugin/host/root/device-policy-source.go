package plugin_host_root

import "context"

// DevicePolicySource exposes daemon-owned policy snapshots without mutation authority.
type DevicePolicySource interface {
	// WaitDevicePolicy returns the current binary policy when it differs from last.
	// A nil last returns the current policy at once. A non-nil last, including
	// the zero-length encoding of the empty policy, waits for a different one.
	WaitDevicePolicy(ctx context.Context, last []byte) (policy []byte, deviceObjectKey string, revision uint64, err error)
}
