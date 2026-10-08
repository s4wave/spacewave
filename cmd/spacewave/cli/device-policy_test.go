//go:build !js

package spacewave_cli

import (
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestDevicePolicyCommandExposesSubcommandsAndFlags(t *testing.T) {
	// Locate every device policy subcommand.
	deviceCmd := newDeviceCommand(nil)
	approveCmd := findTestSubcommand(t, deviceCmd, "approve")
	policyCmd := findTestSubcommand(t, deviceCmd, "policy")

	// Locate the node-type subcommands.
	nodeTypeCmd := findTestSubcommand(t, policyCmd, "node-type")
	nodeTypeAddCmd := findTestSubcommand(t, nodeTypeCmd, "add")
	nodeTypeRemoveCmd := findTestSubcommand(t, nodeTypeCmd, "remove")

	// Check the flags of each located subcommand.
	assertCommandFlags(t, approveCmd, "state-path", "socket-path", "session-index", "space", "ticket")
	assertCommandFlags(t, findTestSubcommand(t, deviceCmd, "show"), "state-path", "socket-path", "session-index", "space", "output")
	assertCommandFlags(t, nodeTypeAddCmd, "state-path", "socket-path")
	assertCommandFlags(t, nodeTypeRemoveCmd, "state-path", "socket-path")
}

func TestProjectDevicePolicyOntoDeviceUpdatesCapabilitiesAndTimestamp(t *testing.T) {
	// Seed the device, policy, and timestamps.
	created := time.Unix(1_700_000_000, 0)
	updated := created.Add(time.Minute)
	now := updated.Add(time.Minute)
	existing := &s4wave_device.Device{
		PeerId:        "peer-device",
		Label:         "build host",
		Platform:      &s4wave_device.DevicePlatform{Os: "linux", Arch: "arm64"},
		DaemonVersion: "0.1.0",
		SetupState:    s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		UpdateState:   s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_IDLE,
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness:   s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
			Message:    "ready",
			ObservedAt: timestamppb.New(updated),
		},
		Capabilities: []*s4wave_device.DeviceCapability{
			{
				Id:    "operator-capability",
				Kind:  "operator",
				Label: "Operator Capability",
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
			},
			{
				Id:   "forge-worker",
				Kind: s4wave_device.DeviceCapabilityKindForgeWorker,
				Policy: &s4wave_device.DeviceCapabilityPolicy{
					LocalPolicyRef: "device-policy/1/forge-worker",
				},
			},
		},
		CreatedAt: timestamppb.New(created),
		UpdatedAt: timestamppb.New(updated),
	}
	policy := &device_policy.DevicePolicy{Revision: 2}

	// Project the policy and check the updated device fields.
	next, changed, err := projectDevicePolicyOntoDevice(existing, policy, now)
	if err != nil {
		t.Fatalf("projectDevicePolicyOntoDevice() error = %v", err)
	}

	// Check the changed flag and the preserved non-policy device fields.
	if !changed {
		t.Fatal("changed = false, want policy capability projection to update device")
	}
	if next.GetPeerId() != existing.GetPeerId() || next.GetLabel() != existing.GetLabel() || next.GetSetupState() != existing.GetSetupState() {
		t.Fatalf("non-policy device fields changed: got peer=%q label=%q setup=%s", next.GetPeerId(), next.GetLabel(), next.GetSetupState())
	}
	if next.GetCreatedAt().GetSeconds() != created.Unix() {
		t.Fatalf("created_at = %v, want %v", next.GetCreatedAt(), timestamppb.New(created))
	}
	if next.GetUpdatedAt().GetSeconds() != now.Unix() {
		t.Fatalf("updated_at = %v, want projection time %v", next.GetUpdatedAt(), timestamppb.New(now))
	}
	byID := deviceCapabilitiesByID(next.GetCapabilities())
	if byID["operator-capability"] == nil || !byID["operator-capability"].EqualVT(existing.GetCapabilities()[0]) {
		t.Fatalf("operator capability = %v, want preserved %v", byID["operator-capability"], existing.GetCapabilities()[0])
	}
	if byID["forge-worker"] != nil {
		t.Fatalf("stale policy-owned capability = %v, want dropped", byID["forge-worker"])
	}
}

func findTestSubcommand(t *testing.T, cmd *cli.Command, name string) *cli.Command {
	t.Helper()
	for _, sub := range cmd.Subcommands {
		if sub.Name == name {
			return sub
		}
	}
	t.Fatalf("subcommand %q missing from %s", name, cmd.Name)
	return nil
}

func deviceCapabilitiesByID(caps []*s4wave_device.DeviceCapability) map[string]*s4wave_device.DeviceCapability {
	byID := make(map[string]*s4wave_device.DeviceCapability, len(caps))
	for _, cap := range caps {
		if cap == nil {
			continue
		}
		byID[cap.GetId()] = cap
	}
	return byID
}
