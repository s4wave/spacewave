package s4wave_device

import (
	"maps"
	"strings"
)

// EffectiveLabels returns a fresh set of Kubernetes-compatible hostname, os and
// arch labels merged with stored labels. Stored values override derived ones.
func (d *Device) EffectiveLabels(deviceKey string) map[string]string {
	// Derive the hostname from the Device object's graph identity, as Node does.
	labels := map[string]string{"kubernetes.io/hostname": strings.TrimPrefix(deviceKey, "devices/")}
	if os := d.GetPlatform().GetOs(); os != "" {
		labels["kubernetes.io/os"] = os
	}
	if arch := d.GetPlatform().GetArch(); arch != "" {
		labels["kubernetes.io/arch"] = arch
	}

	// Copy the stored overrides so callers may edit their own returned map.
	maps.Copy(labels, d.GetLabels())
	return labels
}
