package plugin_host_root

import "context"

// DevicePolicySource exposes daemon-owned policy snapshots without mutation authority.
type DevicePolicySource interface {
	// WaitDevicePolicy returns the current binary policy when it differs from last.
	WaitDevicePolicy(ctx context.Context, last []byte) (policy []byte, deviceObjectKey string, revision uint64, err error)
}
