package forge_runtime

import (
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func testSelectableDevice() *s4wave_device.Device {
	return &s4wave_device.Device{
		PeerId:     "12D3KooWPeer",
		Label:      "bench-1",
		SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*s4wave_device.DeviceCapability{
			{
				Id:    "cap-fs",
				Kind:  s4wave_device.DeviceCapabilityKindFilesystem,
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			},
			{
				Id:    "cap-worker",
				Kind:  s4wave_device.DeviceCapabilityKindForgeWorker,
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
				Link: &s4wave_device.DeviceCapabilityLink{
					ObjectKey: "worker/bench-1",
					TypeId:    "forge/worker",
				},
			},
		},
	}
}

func TestSelectForgeWorkerCapabilityUsesTypedSelection(t *testing.T) {
	// Select the Forge Worker capability from a ready Device.
	dev := testSelectableDevice()
	capability := SelectForgeWorkerCapability(dev)
	if capability == nil || ForgeWorkerObjectKey(capability) != "worker/bench-1" {
		t.Fatalf("unexpected capability: %+v", capability)
	}

	// Check that a Device awaiting setup cannot supply a Worker.
	notReady := testSelectableDevice()
	notReady.SetupState = s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_WAITING_FOR_COMPLETION
	if SelectForgeWorkerCapability(notReady) != nil {
		t.Fatal("non-selectable device must not resolve a worker")
	}

	// Check that a Device without a Worker capability cannot supply one.
	noWorker := testSelectableDevice()
	noWorker.Capabilities = noWorker.Capabilities[:1]
	if SelectForgeWorkerCapability(noWorker) != nil {
		t.Fatal("device without forge-worker capability must not resolve a worker")
	}

	// Check that an absent Device cannot supply a Worker.
	if SelectForgeWorkerCapability(nil) != nil {
		t.Fatal("nil device must not resolve a worker")
	}
}
