//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// TestDevicePolicyHostSourceForwardsEnrolledIdentityAndRemoval checks the
// daemon's actual policy snapshot and setup key before the Worker consumes it.
func TestDevicePolicyHostSourceForwardsEnrolledIdentityAndRemoval(t *testing.T) {
	statePath := t.TempDir()
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState: deviceSetupStateSessionReady, PeerID: "peer", ResourceID: "resource",
		SessionIndex: 1, DeviceObjectKey: "devices/self",
	}); err != nil {
		t.Fatal(err)
	}
	initial := &device_policy.DevicePolicy{
		Revision: 1,
		ForgeWorker: &device_policy.ForgeWorkerPolicy{
			WorkerObjectKey: "worker/test", MilliCpu: 1000, MemoryBytes: 1 << 20,
			Backends: []string{"docker"},
		},
	}
	if err := device_policy.WriteFile(statePath, initial); err != nil {
		t.Fatal(err)
	}
	store, err := device_policy.NewPolicyStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	source := &devicePolicyHostSource{store: store, statePath: statePath}
	data, key, revision, err := source.WaitDevicePolicy(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &device_policy.DevicePolicy{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if key != "devices/self" || revision != 1 || decoded.GetForgeWorker().GetWorkerObjectKey() != "worker/test" {
		t.Fatalf("current policy: key=%q revision=%d policy=%+v", key, revision, decoded)
	}
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	data, key, revision, err = source.WaitDevicePolicy(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	decoded = &device_policy.DevicePolicy{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if key != "devices/self" || revision != 2 || decoded.GetForgeWorker() != nil {
		t.Fatalf("removed policy: key=%q revision=%d policy=%+v", key, revision, decoded)
	}
}
