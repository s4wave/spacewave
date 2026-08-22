package s4wave_device

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// DeviceTypeID is the type identifier for Spacewave-managed Device objects.
const DeviceTypeID = "spacewave/device"

const (
	// DeviceCapabilityKindFilesystem identifies filesystem access exposed by a Device.
	DeviceCapabilityKindFilesystem = "filesystem"
	// DeviceCapabilityKindForgeWorker identifies Forge Worker execution exposed by a Device.
	DeviceCapabilityKindForgeWorker = "forge-worker"
	// DeviceCapabilityKindTerminal identifies terminal access exposed by a Device.
	DeviceCapabilityKindTerminal = "terminal"
)

// NewDeviceBlock constructs a new Device block.
func NewDeviceBlock() block.Block {
	return &Device{}
}

// UnmarshalDevice unmarshals a Device from a cursor.
func UnmarshalDevice(ctx context.Context, bcs *block.Cursor) (*Device, error) {
	return block.UnmarshalBlock[*Device](ctx, bcs, NewDeviceBlock)
}

// MarshalBlock marshals the Device to bytes.
func (d *Device) MarshalBlock() ([]byte, error) {
	return d.MarshalVT()
}

// UnmarshalBlock unmarshals the Device from bytes.
func (d *Device) UnmarshalBlock(data []byte) error {
	return d.UnmarshalVT(data)
}

// Validate performs cursory checks on the Device block.
func (d *Device) Validate() error {
	if strings.TrimSpace(d.GetPeerId()) == "" {
		return errors.New("device peer_id is required")
	}
	if strings.TrimSpace(d.GetLabel()) == "" {
		return errors.New("device label is required")
	}
	seenCapabilities := make(map[string]struct{}, len(d.GetCapabilities()))
	for _, cap := range d.GetCapabilities() {
		if cap == nil {
			continue
		}
		id := strings.TrimSpace(cap.GetId())
		if id == "" {
			return errors.New("device capability id is required")
		}
		if _, ok := seenCapabilities[id]; ok {
			return errors.Errorf("duplicate device capability id %q", id)
		}
		seenCapabilities[id] = struct{}{}
		link := cap.GetLink()
		if link.GetObjectKey() != "" && strings.TrimSpace(link.GetTypeId()) == "" {
			return errors.Errorf("device capability %q link type_id is required with object_key", id)
		}
		if checkoutRoot := cap.GetCheckoutRoot(); checkoutRoot != nil {
			if strings.TrimSpace(cap.GetKind()) != DeviceCapabilityKindFilesystem {
				return errors.Errorf("device capability %q checkout_root requires filesystem kind", id)
			}
			if strings.TrimSpace(checkoutRoot.GetName()) == "" {
				return errors.Errorf("device capability %q checkout_root name is required", id)
			}
			if checkoutRoot.GetWriteAvailable() && !checkoutRoot.GetReadAvailable() {
				return errors.Errorf("device capability %q checkout_root write availability requires read availability", id)
			}
			switch checkoutRoot.GetAccess() {
			case DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY:
				if !checkoutRoot.GetReadAvailable() || checkoutRoot.GetWriteAvailable() {
					return errors.Errorf("device capability %q checkout_root read-only access requires read-only availability", id)
				}
			case DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE:
				if !checkoutRoot.GetReadAvailable() || !checkoutRoot.GetWriteAvailable() {
					return errors.Errorf("device capability %q checkout_root read-write access requires read-write availability", id)
				}
			default:
				if checkoutRoot.GetReadAvailable() || checkoutRoot.GetWriteAvailable() {
					return errors.Errorf("device capability %q checkout_root access is required when availability is set", id)
				}
			}
		}
	}
	return nil
}

// IsSelectable reports whether the Device has enough identity and setup state
// for Forge or a workflow builder to present it as an execution or resource
// target.
func (d *Device) IsSelectable() bool {
	if d == nil {
		return false
	}
	return strings.TrimSpace(d.GetPeerId()) != "" &&
		strings.TrimSpace(d.GetLabel()) != "" &&
		d.GetSetupState() == DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY
}

// FindCapabilityByKind returns the first capability matching the requested kind.
func (d *Device) FindCapabilityByKind(kind string) *DeviceCapability {
	if d == nil {
		return nil
	}
	for _, cap := range d.GetCapabilities() {
		if cap == nil {
			continue
		}
		if strings.TrimSpace(cap.GetKind()) == kind {
			return cap
		}
	}
	return nil
}

// DeviceCapabilityIsSelectable reports whether a capability can be selected by
// Forge or a workflow builder without taking over the capability's execution
// state.
func DeviceCapabilityIsSelectable(cap *DeviceCapability) bool {
	if cap == nil {
		return false
	}
	switch cap.GetState() {
	case DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
		DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE:
		return true
	default:
		return false
	}
}

// DeviceCapabilityPolicyAllowsWrite reports whether the projected policy state
// permits a caller-approved write escalation.
func DeviceCapabilityPolicyAllowsWrite(cap *DeviceCapability) bool {
	if cap == nil {
		return false
	}
	policy := cap.GetPolicy()
	return policy.GetLocalState() == DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED &&
		policy.GetGrantState() == DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED
}

// HasSelectableCapabilityKind reports whether the Device exposes a selectable
// capability of the requested kind.
func (d *Device) HasSelectableCapabilityKind(kind string) bool {
	if d == nil {
		return false
	}
	for _, cap := range d.GetCapabilities() {
		if cap == nil || strings.TrimSpace(cap.GetKind()) != kind {
			continue
		}
		if DeviceCapabilityIsSelectable(cap) {
			return true
		}
	}
	return false
}

// FindSelectableCheckoutRoot returns a selectable filesystem capability for a
// named checkout root. An empty name selects the first selectable checkout root.
func (d *Device) FindSelectableCheckoutRoot(name string) *DeviceCapability {
	if d == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	for _, cap := range d.GetCapabilities() {
		if cap == nil || strings.TrimSpace(cap.GetKind()) != DeviceCapabilityKindFilesystem {
			continue
		}
		if !DeviceCapabilityIsSelectable(cap) {
			continue
		}
		checkoutRoot := cap.GetCheckoutRoot()
		if checkoutRoot == nil {
			continue
		}
		if name == "" || strings.TrimSpace(checkoutRoot.GetName()) == name {
			return cap
		}
	}
	return nil
}

// DeviceCheckoutRootCanRead reports whether a checkout root may be opened for
// reads through its linked filesystem object.
func DeviceCheckoutRootCanRead(checkoutRoot *DeviceCheckoutRootCapability) bool {
	if checkoutRoot == nil || !checkoutRoot.GetReadAvailable() {
		return false
	}
	switch checkoutRoot.GetAccess() {
	case DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
		DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE:
		return true
	default:
		return false
	}
}

// DeviceCheckoutRootCanWrite reports whether a checkout root may be written
// after the caller obtains the required approval.
func DeviceCheckoutRootCanWrite(checkoutRoot *DeviceCheckoutRootCapability) bool {
	return checkoutRoot != nil &&
		checkoutRoot.GetAccess() == DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE &&
		checkoutRoot.GetReadAvailable() &&
		checkoutRoot.GetWriteAvailable()
}

// DeviceCapabilityCanWriteCheckoutRoot reports whether a capability can pass
// the checkout-root write gate after a caller supplies approval.
func DeviceCapabilityCanWriteCheckoutRoot(cap *DeviceCapability) bool {
	return DeviceCheckoutRootCanWrite(cap.GetCheckoutRoot()) &&
		DeviceCapabilityPolicyAllowsWrite(cap)
}

// FindReadableCheckoutRoot returns a selectable checkout-root capability with a
// linked filesystem object that can be opened through the Resource SDK.
func (d *Device) FindReadableCheckoutRoot(name string) *DeviceCapability {
	cap := d.FindSelectableCheckoutRoot(name)
	if cap == nil || !DeviceCheckoutRootCanRead(cap.GetCheckoutRoot()) {
		return nil
	}
	link := cap.GetLink()
	if strings.TrimSpace(link.GetObjectKey()) == "" || strings.TrimSpace(link.GetTypeId()) == "" {
		return nil
	}
	return cap
}

// FindWritableCheckoutRoot returns a selectable checkout-root capability with a
// linked filesystem object that may be opened for writes after approval.
func (d *Device) FindWritableCheckoutRoot(name string) *DeviceCapability {
	cap := d.FindSelectableCheckoutRoot(name)
	if cap == nil || !DeviceCapabilityCanWriteCheckoutRoot(cap) {
		return nil
	}
	link := cap.GetLink()
	if strings.TrimSpace(link.GetObjectKey()) == "" || strings.TrimSpace(link.GetTypeId()) == "" {
		return nil
	}
	return cap
}

// FindSelectableForgeWorker returns a selectable Forge Worker capability linked
// to the Worker object that records execution state, logs, and results.
func (d *Device) FindSelectableForgeWorker() *DeviceCapability {
	if d == nil {
		return nil
	}
	for _, cap := range d.GetCapabilities() {
		if cap == nil || strings.TrimSpace(cap.GetKind()) != DeviceCapabilityKindForgeWorker {
			continue
		}
		if !DeviceCapabilityIsSelectable(cap) {
			continue
		}
		link := cap.GetLink()
		if strings.TrimSpace(link.GetObjectKey()) == "" || strings.TrimSpace(link.GetTypeId()) == "" {
			continue
		}
		return cap
	}
	return nil
}

var _ block.Block = (*Device)(nil)
