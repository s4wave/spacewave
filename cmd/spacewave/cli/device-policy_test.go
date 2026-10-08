//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"net"
	"strings"
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

	// Locate the forge-worker subcommands.
	forgeWorkerCmd := findTestSubcommand(t, policyCmd, "forge-worker")
	forgeWorkerSetCmd := findTestSubcommand(t, forgeWorkerCmd, "set")
	forgeWorkerShowCmd := findTestSubcommand(t, forgeWorkerCmd, "show")
	forgeWorkerClearCmd := findTestSubcommand(t, forgeWorkerCmd, "clear")

	// Locate the node-type subcommands.
	nodeTypeCmd := findTestSubcommand(t, policyCmd, "node-type")
	nodeTypeAddCmd := findTestSubcommand(t, nodeTypeCmd, "add")
	nodeTypeRemoveCmd := findTestSubcommand(t, nodeTypeCmd, "remove")

	// Check the flags of each located subcommand.
	assertCommandFlags(t, approveCmd, "state-path", "socket-path", "session-index", "space", "ticket")
	assertCommandFlags(t, forgeWorkerSetCmd, "state-path", "socket-path", "milli-cpu", "memory-bytes", "backend")
	assertCommandFlags(t, forgeWorkerShowCmd, "state-path", "output")
	assertCommandFlags(t, forgeWorkerClearCmd, "state-path", "socket-path")
	assertCommandFlags(t, findTestSubcommand(t, deviceCmd, "show"), "state-path", "socket-path", "session-index", "space", "output")
	assertCommandFlags(t, nodeTypeAddCmd, "state-path", "socket-path")
	assertCommandFlags(t, nodeTypeRemoveCmd, "state-path", "socket-path")
}

func TestComputeDevicePolicyCapabilitiesProjectsPolicyOwnedCapabilities(t *testing.T) {
	// Seed the existing capabilities with an operator-owned entry and a
	// Flowgraph node capability of the filesystem kind.
	nodeCap := &s4wave_device.DeviceCapability{
		Id:    s4wave_device.DeviceCapabilityKindFlowgraphNode + "/graph/skiffos",
		Kind:  s4wave_device.DeviceCapabilityKindFilesystem,
		Label: "skiffos checkout",
		State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
	}
	existing := []*s4wave_device.DeviceCapability{
		nodeCap,
		{
			Id:     "custom-capability",
			Kind:   "custom",
			Label:  "Operator Capability",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
			Detail: "operator-owned",
			Link:   &s4wave_device.DeviceCapabilityLink{ProtocolId: "spacewave/custom"},
		},
		{
			Id:     devicePolicyForgeWorkerCapabilityID,
			Kind:   s4wave_device.DeviceCapabilityKindForgeWorker,
			Label:  "old worker label",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Detail: "Space denied worker",
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "device-policy/12/forge-worker",
				GrantPolicyRef: "grant/forge-worker",
				LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
			},
		},
		{
			Id:    "remote-shell",
			Kind:  "remote-shell",
			Label: "Remote Shell",
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "device-policy/12/remote-shell",
				LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			},
		},
	}
	policy := &device_policy.DevicePolicy{
		Revision:    13,
		ForgeWorker: &device_policy.ForgeWorkerPolicy{WorkerObjectKey: "worker/1"},
	}

	// Project the policy onto the existing capabilities and index them.
	got := computeDevicePolicyCapabilities(policy, existing)
	byID := deviceCapabilitiesByID(got)

	// Check the capabilities keep their order with the node capability last.
	if len(got) != 3 {
		t.Fatalf("capability count = %d, want non-policy + forge worker + node", len(got))
	}
	if got[len(got)-1].GetId() != nodeCap.GetId() || !got[len(got)-1].EqualVT(nodeCap) {
		t.Fatalf("last capability = %v, want preserved node capability %v", got[len(got)-1], nodeCap)
	}
	nonPolicy := byID["custom-capability"]
	if nonPolicy == nil || !nonPolicy.EqualVT(existing[1]) {
		t.Fatalf("non-policy capability = %v, want preserved %v", nonPolicy, existing[1])
	}

	// Check the capability the policy wrote for a setting it no longer has is gone.
	if retired := byID["remote-shell"]; retired != nil {
		t.Fatalf("retired remote-shell capability = %v, want dropped", retired)
	}

	// Check the projected forge-worker capability.
	forgeWorker := byID[devicePolicyForgeWorkerCapabilityID]
	if forgeWorker == nil {
		t.Fatal("forge-worker capability missing")
	}
	if forgeWorker.GetPolicy().GetLocalPolicyRef() != "device-policy/13/forge-worker" {
		t.Fatalf("forge-worker local policy ref = %q", forgeWorker.GetPolicy().GetLocalPolicyRef())
	}
	if forgeWorker.GetPolicy().GetGrantPolicyRef() != "grant/forge-worker" {
		t.Fatalf("forge-worker grant policy ref = %q", forgeWorker.GetPolicy().GetGrantPolicyRef())
	}
	if forgeWorker.GetPolicy().GetGrantState() != s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED {
		t.Fatalf("forge-worker grant state = %s", forgeWorker.GetPolicy().GetGrantState())
	}
	if forgeWorker.GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED {
		t.Fatalf("forge-worker state = %s", forgeWorker.GetState())
	}
	if forgeWorker.GetDetail() != "Space denied worker" {
		t.Fatalf("forge-worker detail = %q", forgeWorker.GetDetail())
	}
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
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id:    "operator-capability",
			Kind:  "operator",
			Label: "Operator Capability",
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED,
		}},
		CreatedAt: timestamppb.New(created),
		UpdatedAt: timestamppb.New(updated),
	}
	policy := &device_policy.DevicePolicy{
		Revision:    2,
		ForgeWorker: &device_policy.ForgeWorkerPolicy{WorkerObjectKey: "worker/1"},
	}

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
	if byID[devicePolicyForgeWorkerCapabilityID] == nil {
		t.Fatal("forge-worker capability missing")
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

func withDevicePolicyReloadStub(t *testing.T, reload func(context.Context, *sdkClient) error) {
	t.Helper()
	oldReload := devicePolicyReloadDaemon
	t.Cleanup(func() {
		devicePolicyReloadDaemon = oldReload
	})
	devicePolicyReloadDaemon = reload
}

func TestDevicePolicyForgeWorkerSetAndClearValidateBeforeWriting(t *testing.T) {
	// Seed the state path and stub the daemon connection.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)
	statePath := t.TempDir()
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 30}); err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	// Stub the daemon connection and reload hooks.
	withDeviceDaemonStub(t, func(string, int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(context.Context, string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})
	var reloads int
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})
	originalValidate := devicePolicyValidateForgeWorker
	t.Cleanup(func() { devicePolicyValidateForgeWorker = originalValidate })
	var validated *device_policy.ForgeWorkerPolicy
	devicePolicyValidateForgeWorker = func(
		_ context.Context,
		gotStatePath string,
		_ *sdkClient,
		policy *device_policy.ForgeWorkerPolicy,
	) error {
		if gotStatePath != statePath {
			t.Fatalf("validation state path = %q, want %q", gotStatePath, statePath)
		}
		validated = policy.CloneVT()
		return nil
	}

	// Run the forge-worker set command and verify the written policy.
	if err := runDeviceCLI(t,
		"device", "policy", "forge-worker", "set",
		"--state-path", statePath,
		"--milli-cpu", "2500",
		"--memory-bytes", "4294967296",
		"--backend", "fuse",
		"--backend", "docker",
		"forge/worker/forge-device",
	); err != nil {
		t.Fatalf("device policy forge-worker set: %v", err)
	}
	if validated == nil || validated.GetWorkerObjectKey() != "forge/worker/forge-device" {
		t.Fatalf("validated policy = %v", validated)
	}
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read set policy: %v", err)
	}
	worker := policy.GetForgeWorker()
	if policy.GetRevision() != 31 || worker.GetMilliCpu() != 2500 || worker.GetMemoryBytes() != 4294967296 {
		t.Fatalf("set policy = %v", policy)
	}
	if got := worker.GetBackends(); len(got) != 2 || got[0] != "docker" || got[1] != "fuse" {
		t.Fatalf("backends = %v, want canonical [docker fuse]", got)
	}

	// Stub the Forge Worker validation hook.
	devicePolicyValidateForgeWorker = func(context.Context, string, *sdkClient, *device_policy.ForgeWorkerPolicy) error {
		return errors.New("wrong Device session keypair")
	}
	if err := runDeviceCLI(t,
		"device", "policy", "forge-worker", "set",
		"--state-path", statePath,
		"--milli-cpu", "1000",
		"--memory-bytes", "1073741824",
		"--backend", "docker",
		"forge/worker/other",
	); err == nil || !strings.Contains(err.Error(), "wrong Device session keypair") {
		t.Fatalf("mismatched Worker error = %v", err)
	}
	unchanged, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.GetRevision() != 31 || unchanged.GetForgeWorker().GetWorkerObjectKey() != "forge/worker/forge-device" {
		t.Fatalf("failed validation changed policy: %v", unchanged)
	}

	// Run the forge-worker set command and verify the written policy.
	if err := runDeviceCLI(t,
		"device", "policy", "forge-worker", "clear", "--state-path", statePath,
	); err != nil {
		t.Fatalf("device policy forge-worker clear: %v", err)
	}
	cleared, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.GetRevision() != 32 || cleared.GetForgeWorker() != nil {
		t.Fatalf("cleared policy = %v", cleared)
	}
	if reloads != 2 {
		t.Fatalf("reloads = %d, want set and clear", reloads)
	}
}

func TestNormalizedForgeWorkerBackendsRejectsUnsafeValues(t *testing.T) {
	for _, values := range [][]string{nil, {""}, {"docker", "docker"}, {"docker,host"}, {"docker host"}} {
		if got, err := normalizedForgeWorkerBackends(values); err == nil || got != nil {
			t.Fatalf("normalizedForgeWorkerBackends(%q) = %v, %v; want error", values, got, err)
		}
	}
}
