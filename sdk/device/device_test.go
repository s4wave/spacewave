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

func TestDeviceSelectableLimaWorkflowTarget(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		LastStatus: &DeviceStatus{
			Liveness: DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:  "device session ready",
		},
		Capabilities: []*DeviceCapability{
			{
				Id:    "checkout-root-skiffos",
				Kind:  DeviceCapabilityKindFilesystem,
				Label: "SkiffOS checkout",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
				Link: &DeviceCapabilityLink{
					ObjectKey: "unixfs/skiffos-checkout",
					TypeId:    "unixfs/fs-node",
				},
				CheckoutRoot: &DeviceCheckoutRootCapability{
					Name:           "skiffos",
					DisplayPath:    "~/repos/skiffos/skiffos",
					SelectionRef:   "device/lima/filesystem/skiffos",
					Access:         DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
					ReadAvailable:  true,
					WriteAvailable: true,
				},
				Policy: &DeviceCapabilityPolicy{
					LocalPolicyRef: "device/lima/filesystem/skiffos",
					GrantPolicyRef: "space/grants/lima/skiffos",
					LocalState:     DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState:     DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			{
				Id:    "forge-worker",
				Kind:  DeviceCapabilityKindForgeWorker,
				Label: "Forge Worker",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
				Link: &DeviceCapabilityLink{
					ObjectKey: "forge/workers/lima",
					TypeId:    "forge/worker",
				},
				Policy: &DeviceCapabilityPolicy{
					LocalPolicyRef: "device/lima/forge-worker",
					GrantPolicyRef: "space/grants/lima/forge-worker",
					LocalState:     DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState:     DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
		},
	}
	if err := dev.Validate(); err != nil {
		t.Fatalf("lima workflow target failed validation: %v", err)
	}
	if !dev.IsSelectable() {
		t.Fatal("expected device to be selectable")
	}
	if !dev.HasSelectableCapabilityKind(DeviceCapabilityKindFilesystem) {
		t.Fatal("expected selectable filesystem capability")
	}
	if !dev.HasSelectableCapabilityKind(DeviceCapabilityKindForgeWorker) {
		t.Fatal("expected selectable forge-worker capability")
	}
	if got := dev.FindSelectableForgeWorker().GetLink().GetObjectKey(); got != "forge/workers/lima" {
		t.Fatalf("expected forge worker object link, got %q", got)
	}
	if got := dev.FindCapabilityByKind(DeviceCapabilityKindFilesystem).GetLink().GetObjectKey(); got != "unixfs/skiffos-checkout" {
		t.Fatalf("expected checkout root object link, got %q", got)
	}
	if got := dev.FindSelectableCheckoutRoot("skiffos").GetCheckoutRoot().GetSelectionRef(); got != "device/lima/filesystem/skiffos" {
		t.Fatalf("expected checkout root selection ref, got %q", got)
	}
	readable := dev.FindReadableCheckoutRoot("skiffos")
	if readable == nil {
		t.Fatal("expected readable checkout root")
	}
	if readable.GetLink().GetTypeId() != "unixfs/fs-node" {
		t.Fatalf("expected unixfs owner link, got %q", readable.GetLink().GetTypeId())
	}
	if !DeviceCheckoutRootCanRead(readable.GetCheckoutRoot()) {
		t.Fatal("expected checkout root to be readable")
	}
	if !DeviceCheckoutRootCanWrite(readable.GetCheckoutRoot()) {
		t.Fatal("expected checkout root to be writable after approval")
	}
	if writable := dev.FindWritableCheckoutRoot("skiffos"); writable == nil {
		t.Fatal("expected checkout root to be write-gateable")
	}
}

func TestDeviceCapabilitySelectionRejectsBlockedStates(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{{
			Id:     "checkout-root-skiffos",
			Kind:   DeviceCapabilityKindFilesystem,
			Label:  "SkiffOS checkout",
			State:  DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED,
			Detail: "blocked by Space grant",
			Link: &DeviceCapabilityLink{
				ObjectKey: "unixfs/skiffos-checkout",
				TypeId:    "unixfs/fs-node",
			},
		}},
	}
	if !dev.IsSelectable() {
		t.Fatal("expected device identity and setup state to remain selectable")
	}
	if dev.HasSelectableCapabilityKind(DeviceCapabilityKindFilesystem) {
		t.Fatal("grant-blocked capability was selectable")
	}
	if dev.FindSelectableCheckoutRoot("skiffos") != nil {
		t.Fatal("grant-blocked checkout root was selectable")
	}
	if dev.FindReadableCheckoutRoot("skiffos") != nil {
		t.Fatal("grant-blocked checkout root was readable")
	}
}

func TestDeviceValidateCheckoutRootShape(t *testing.T) {
	err := (&Device{
		PeerId: "peer-device",
		Label:  "Device",
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindTerminal,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:          "skiffos",
				ReadAvailable: true,
			},
		}},
	}).Validate()
	if err == nil {
		t.Fatal("Validate accepted checkout_root on non-filesystem capability")
	}

	err = (&Device{
		PeerId: "peer-device",
		Label:  "Device",
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:           "skiffos",
				WriteAvailable: true,
			},
		}},
	}).Validate()
	if err == nil {
		t.Fatal("Validate accepted checkout_root write availability without read availability")
	}

	err = (&Device{
		PeerId: "peer-device",
		Label:  "Device",
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:          "skiffos",
				ReadAvailable: true,
			},
		}},
	}).Validate()
	if err == nil {
		t.Fatal("Validate accepted checkout_root availability without access mode")
	}

	err = (&Device{
		PeerId: "peer-device",
		Label:  "Device",
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:           "skiffos",
				Access:         DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
				ReadAvailable:  true,
				WriteAvailable: true,
			},
		}},
	}).Validate()
	if err == nil {
		t.Fatal("Validate accepted read-only checkout_root access with write availability")
	}
}

func TestDeviceCheckoutRootAccessGatesAvailability(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link: &DeviceCapabilityLink{
				ObjectKey: "unixfs/skiffos-checkout",
				TypeId:    "unixfs/fs-node",
			},
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:          "skiffos",
				ReadAvailable: true,
			},
		}},
	}
	if DeviceCheckoutRootCanRead(dev.GetCapabilities()[0].GetCheckoutRoot()) {
		t.Fatal("unknown checkout_root access was readable")
	}
	if dev.FindReadableCheckoutRoot("skiffos") != nil {
		t.Fatal("unknown checkout_root access produced readable owner link")
	}
}

func TestDeviceReadableCheckoutRootRequiresOwnerLink(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:          "skiffos",
				Access:        DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
				ReadAvailable: true,
			},
		}},
	}
	if dev.FindSelectableCheckoutRoot("skiffos") == nil {
		t.Fatal("expected checkout root to remain selectable")
	}
	if dev.FindReadableCheckoutRoot("skiffos") != nil {
		t.Fatal("checkout root without owner link was readable")
	}
}

func TestDeviceCapabilityPolicyRejectsUnavailablePocInputs(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{
			{
				Id:    "checkout-root-skiffos",
				Kind:  DeviceCapabilityKindFilesystem,
				Label: "SkiffOS checkout",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED,
				Link: &DeviceCapabilityLink{
					ObjectKey: "unixfs/skiffos-checkout",
					TypeId:    "unixfs/fs-node",
				},
				CheckoutRoot: &DeviceCheckoutRootCapability{
					Name:          "skiffos",
					Access:        DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
					ReadAvailable: true,
				},
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_DISABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
				},
			},
			{
				Id:    "forge-worker",
				Kind:  DeviceCapabilityKindForgeWorker,
				Label: "Forge Worker",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
				Link: &DeviceCapabilityLink{
					ObjectKey: "forge/workers/lima",
					TypeId:    "forge/worker",
				},
				Policy: &DeviceCapabilityPolicy{
					LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
					GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_UNKNOWN,
				},
			},
		},
	}
	if dev.FindReadableCheckoutRoot("skiffos") != nil {
		t.Fatal("disabled checkout root was readable")
	}
	if dev.FindWritableCheckoutRoot("skiffos") != nil {
		t.Fatal("disabled checkout root was writable")
	}
	if dev.FindSelectableForgeWorker() != nil {
		t.Fatal("declared forge worker was selectable")
	}
}

func TestDeviceWritableCheckoutRootRequiresAllowedPolicy(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{{
			Id:    "checkout-root-skiffos",
			Kind:  DeviceCapabilityKindFilesystem,
			Label: "SkiffOS checkout",
			State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link: &DeviceCapabilityLink{
				ObjectKey: "unixfs/skiffos-checkout",
				TypeId:    "unixfs/fs-node",
			},
			CheckoutRoot: &DeviceCheckoutRootCapability{
				Name:           "skiffos",
				Access:         DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
				ReadAvailable:  true,
				WriteAvailable: true,
			},
			Policy: &DeviceCapabilityPolicy{
				LocalState: DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState: DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
			},
		}},
	}
	if dev.FindReadableCheckoutRoot("skiffos") == nil {
		t.Fatal("expected blocked write policy to keep checkout root readable")
	}
	if dev.FindWritableCheckoutRoot("skiffos") != nil {
		t.Fatal("grant-blocked checkout root was writable")
	}
}

func TestDeviceCapabilitySelectionSkipsBlockedSameKind(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{
			{
				Id:    "checkout-root-blocked",
				Kind:  DeviceCapabilityKindFilesystem,
				Label: "Blocked checkout",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED,
			},
			{
				Id:    "checkout-root-skiffos",
				Kind:  DeviceCapabilityKindFilesystem,
				Label: "SkiffOS checkout",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			},
		},
	}
	if !dev.HasSelectableCapabilityKind(DeviceCapabilityKindFilesystem) {
		t.Fatal("expected later selectable filesystem capability")
	}
}

func TestDeviceFindSelectableCapabilityByID(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWDevice",
		Label:      "Build Host",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{
			{
				Id:    "glados",
				Kind:  "sensor",
				Label: "Fixture Capability",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
				Link:  &DeviceCapabilityLink{ProtocolId: "plugin/test-plugin/fixture/v0"},
			},
			{
				Id:    "blocked-sensor",
				Kind:  "sensor",
				Label: "Blocked Capability",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED,
				Link:  &DeviceCapabilityLink{ProtocolId: "plugin/test-plugin/blocked/v0"},
			},
		},
	}

	got := dev.FindSelectableCapability("glados")
	if got == nil || got.GetId() != "glados" {
		t.Fatalf("expected glados capability, got %v", got)
	}
	if dev.FindSelectableCapability(" blocked-sensor ") != nil {
		t.Fatal("blocked capability must not be selectable by id")
	}
	if dev.FindSelectableCapability("") != nil {
		t.Fatal("empty id must not select any capability")
	}
	if dev.FindSelectableCapability("plugin/test-plugin/fixture/v0") != nil {
		t.Fatal("raw protocol id must not select a capability")
	}
	if (*Device)(nil).FindSelectableCapability("glados") != nil {
		t.Fatal("nil device must not select a capability")
	}
}

func TestDeviceForgeWorkerSelectionSkipsBlockedSameKind(t *testing.T) {
	dev := &Device{
		PeerId:     "12D3KooWLima",
		Label:      "lima",
		SetupState: DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*DeviceCapability{
			{
				Id:    "forge-worker-blocked",
				Kind:  DeviceCapabilityKindForgeWorker,
				Label: "Blocked Forge Worker",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DISABLED,
				Link: &DeviceCapabilityLink{
					ObjectKey: "forge/workers/blocked",
					TypeId:    "forge/worker",
				},
			},
			{
				Id:    "forge-worker",
				Kind:  DeviceCapabilityKindForgeWorker,
				Label: "Forge Worker",
				State: DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
				Link: &DeviceCapabilityLink{
					ObjectKey: "forge/workers/lima",
					TypeId:    "forge/worker",
				},
			},
		},
	}
	if got := dev.FindSelectableForgeWorker().GetLink().GetObjectKey(); got != "forge/workers/lima" {
		t.Fatalf("expected later selectable forge worker link, got %q", got)
	}
}
