package s4wave_device

import "testing"

func TestDeviceValidatePinsIdentityAndCapabilityIds(t *testing.T) {
	dev := &Device{
		PeerId: "12D3KooWDevice",
		Label:  "Build Host",
		Capabilities: []*DeviceCapability{
			{Id: "filesystem", Kind: "filesystem", Label: "Files"},
			{Id: "forge-worker", Kind: "forge-worker", Label: "Forge Worker"},
		},
	}
	if err := dev.Validate(); err != nil {
		t.Fatalf("valid device failed validation: %v", err)
	}

	dev.PeerId = ""
	if err := dev.Validate(); err == nil {
		t.Fatal("expected missing peer_id to fail validation")
	}

	dev.PeerId = "12D3KooWDevice"
	dev.Capabilities = append(dev.Capabilities, &DeviceCapability{Id: "filesystem"})
	if err := dev.Validate(); err == nil {
		t.Fatal("expected duplicate capability id to fail validation")
	}
}

func TestDeviceValidateCapabilityLinkRequiresType(t *testing.T) {
	err := (&Device{
		PeerId: "peer-device",
		Label:  "Device",
		Capabilities: []*DeviceCapability{{
			Id:   "filesystem",
			Kind: "filesystem",
			Link: &DeviceCapabilityLink{ObjectKey: "files/root"},
		}},
	}).Validate()
	if err == nil {
		t.Fatal("Validate accepted capability object link without type_id")
	}
}
