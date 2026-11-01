//go:build !js

package spacewave_cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// TestDeviceShowAvailability checks text and JSON claim diagnostics independently
// of the Device's last self-reported ONLINE status.
func TestDeviceShowAvailability(t *testing.T) {
	// Show each derived state with the same Device diagnostic record.
	device := &s4wave_device.Device{
		PeerId: "peer-device",
		Label:  "Build host",
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness: s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
		},
	}
	expiry := time.Date(2026, time.October, 8, 20, 0, 0, 0, time.UTC)
	for _, availability := range []forge_runtime.DeviceAvailability{
		{
			State:      forge_runtime.DeviceAvailable,
			ClaimEpoch: 7,
			ExpiresAt:  expiry,
			OwnerState: forge_runtime.CapacityOwnerStateActive,
			Reason:     "forge worker owner claim is active",
		},
		{
			State:      forge_runtime.DeviceDraining,
			ClaimEpoch: 7,
			ExpiresAt:  expiry,
			OwnerState: forge_runtime.CapacityOwnerStateDraining,
			Reason:     "forge worker owner claim is draining",
		},
		{
			State:      forge_runtime.DeviceUnavailable,
			ClaimEpoch: 7,
			ExpiresAt:  expiry,
			OwnerState: forge_runtime.CapacityOwnerStateActive,
			Reason:     "forge worker owner claim expired",
		},
		{
			State:  forge_runtime.DeviceUnavailable,
			Reason: "no forge worker declaration",
		},
	} {
		t.Run(availability.Reason, func(t *testing.T) {
			// Preserve the generated Device fields while adding the derived summary.
			data, err := marshalDeviceShow("devices/self", device, availability)
			if err != nil {
				t.Fatal(err)
			}
			var parser fastjson.Parser
			doc, err := parser.ParseBytes(data)
			if err != nil {
				t.Fatal(err)
			}
			if string(doc.GetStringBytes("label")) != device.Label ||
				doc.GetInt("lastStatus", "liveness") != int(device.LastStatus.Liveness) ||
				doc.GetUint64("availability", "claimEpoch") != availability.ClaimEpoch {
				t.Fatalf("Device JSON lost identity, liveness or claim epoch: %s", data)
			}

			// Check every human-readable availability value in the JSON document.
			values := map[string]string{
				"state":      string(availability.State),
				"expiresAt":  formatDeviceClaimExpiry(availability.ExpiresAt),
				"ownerState": formatDeviceOwnerState(availability.OwnerState),
				"reason":     availability.Reason,
			}
			for key, want := range values {
				if got := string(doc.GetStringBytes("availability", key)); got != want {
					t.Fatalf("availability.%s = %q, want %q", key, got, want)
				}
			}

			// Render the production text writer and compare its aligned diagnostic rows.
			var text bytes.Buffer
			writeDeviceShow(&text, "devices/self", device, availability)
			for _, row := range []string{
				"Availability:     " + string(availability.State),
				"Claim epoch:      " + strconv.FormatUint(availability.ClaimEpoch, 10),
				"Claim expiry:     " + values["expiresAt"],
				"Owner state:      " + values["ownerState"],
				"Reason:           " + availability.Reason,
				"Last liveness:    ONLINE",
			} {
				if !strings.Contains(text.String(), row+"\n") {
					t.Fatalf("missing row %q in:\n%s", row, text.String())
				}
			}
		})
	}
}
