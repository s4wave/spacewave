package forge_runtime

import (
	"strings"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// SelectForgeWorkerCapability resolves one enrolled Device to its selectable
// Forge Worker capability using the existing typed selection APIs. Device
// selection changes the Worker that runs the same manifest; it never creates
// a Device-specific session path or a second scheduler.
func SelectForgeWorkerCapability(dev *s4wave_device.Device) *s4wave_device.DeviceCapability {
	if dev == nil || !dev.IsSelectable() {
		return nil
	}
	return dev.FindSelectableForgeWorker()
}

// ForgeWorkerObjectKey returns the linked Worker object key for a Forge
// Worker capability, empty when the capability has no linked Worker.
func ForgeWorkerObjectKey(capability *s4wave_device.DeviceCapability) string {
	if capability == nil {
		return ""
	}
	return strings.TrimSpace(capability.GetLink().GetObjectKey())
}
