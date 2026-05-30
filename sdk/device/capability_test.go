package s4wave_device

import "testing"

func TestNormalizeCapabilityState(t *testing.T) {
	tests := []struct {
		name       string
		capability *DeviceCapability
		wantState  DeviceCapabilityState
		wantDetail string
	}{
		{
			name: "declared default",
			capability: &DeviceCapability{
				Id:    "terminal",
				Kind:  "terminal",
				Label: "Terminal",
			},
			wantState: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
		},
		{
			name: "available from local and grant policy",
			capability: &DeviceCapability{
				Id:    "filesystem",
				Kind:  "filesystem",
				Label: "Files",
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			wantState: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
		},
		{
			name: "disabled by local policy",
			capability: &DeviceCapability{
				Id:    "terminal",
				Kind:  "terminal",
				Label: "Terminal",
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_DISABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			wantState:  DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED,
			wantDetail: "disabled by local policy",
		},
		{
			name: "blocked by Space grant",
			capability: &DeviceCapability{
				Id:    "forge-worker",
				Kind:  "forge-worker",
				Label: "Forge Worker",
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
				},
			},
			wantState:  DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED,
			wantDetail: "blocked by Space grant",
		},
		{
			name: "active when policy allows",
			capability: &DeviceCapability{
				Id:    "terminal",
				Kind:  "terminal",
				Label: "Terminal",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			wantState: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCapability(tt.capability)
			if got.GetState() != tt.wantState {
				t.Fatalf("state = %v, want %v", got.GetState(), tt.wantState)
			}
			if got.GetDetail() != tt.wantDetail {
				t.Fatalf("detail = %q, want %q", got.GetDetail(), tt.wantDetail)
			}
		})
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
