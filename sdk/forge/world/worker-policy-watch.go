//go:build !tinygo && !goscript && !js

package s4wave_forge_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// workerPolicyWatch follows the daemon policy bound to the native host Root.
type workerPolicyWatch struct {
	// ctx ends pending waits when the watch closes.
	ctx context.Context
	// source supplies the current and changed policy snapshots.
	source plugin_host_root.DevicePolicySource
	// last is the most recently received encoded policy.
	last []byte
	// release cancels ctx and releases the Root reference.
	release func()
}

// openWorkerPolicyWatch looks up the native host Root, which the Space runtime
// bridges into its generation, and waits for the daemon to bind its policy.
func openWorkerPolicyWatch(ctx context.Context, b bus.Bus) (*workerPolicyWatch, error) {
	// Find the Root that owns this process's daemon policy.
	ctx, cancel := context.WithCancel(ctx)
	root, _, rootRef, err := plugin_host_root.ExLookupRootByPlatform(
		ctx, b, false, (&bldr_platform.NativePlatform{}).GetPlatformID(), nil,
	)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "look up native host root")
	}

	// Wait for the daemon to bind its policy source.
	source, err := root.WaitDevicePolicySource(ctx)
	if err != nil {
		rootRef.Release()
		cancel()
		return nil, err
	}
	return &workerPolicyWatch{ctx: ctx, source: source, release: func() {
		cancel()
		rootRef.Release()
	}}, nil
}

// Recv waits for the first or next policy snapshot and its enrolled Device identity.
func (w *workerPolicyWatch) Recv() (*device_policy.DevicePolicy, string, error) {
	// Wait for a revision other than the last one received.
	data, deviceKey, revision, err := w.source.WaitDevicePolicy(w.ctx, w.last)
	if err != nil {
		return nil, "", err
	}

	// Decode it and check it against the reported revision.
	policy := &device_policy.DevicePolicy{}
	if err := policy.UnmarshalVT(data); err != nil {
		return nil, "", errors.Wrap(err, "decode daemon policy")
	}
	if policy.GetRevision() != revision {
		return nil, "", errors.New("device policy revision mismatch")
	}
	w.last = data
	return policy, deviceKey, nil
}

// Close ends pending waits and releases the Root reference.
func (w *workerPolicyWatch) Close() { w.release() }
