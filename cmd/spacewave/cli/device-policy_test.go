//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"net"
	"path/filepath"
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
	enableShellCmd := findTestSubcommand(t, policyCmd, "enable-shell")

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
	assertCommandFlags(t, enableShellCmd, "state-path", "socket-path", "disable")
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
			Id:     devicePolicyRemoteShellCapabilityID,
			Kind:   devicePolicyRemoteShellCapabilityKind,
			Label:  "old shell label",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Detail: "Space denied terminal",
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "device-policy/12/remote-shell",
				GrantPolicyRef: "grant/remote-shell",
				LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED,
			},
		},
	}
	policy := &device_policy.DevicePolicy{
		Revision:    13,
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true, Detail: "terminal enabled"},
	}

	// Project the policy onto the existing capabilities and index them.
	got := computeDevicePolicyCapabilities(policy, existing)
	byID := deviceCapabilitiesByID(got)

	// Check the capabilities keep their order with the node capability last.
	if len(got) != 3 {
		t.Fatalf("capability count = %d, want non-policy + remote shell + node", len(got))
	}
	if got[len(got)-1].GetId() != nodeCap.GetId() || !got[len(got)-1].EqualVT(nodeCap) {
		t.Fatalf("last capability = %v, want preserved node capability %v", got[len(got)-1], nodeCap)
	}
	nonPolicy := byID["custom-capability"]
	if nonPolicy == nil || !nonPolicy.EqualVT(existing[1]) {
		t.Fatalf("non-policy capability = %v, want preserved %v", nonPolicy, existing[1])
	}

	// Check the projected remote-shell capability.
	remoteShell := byID[devicePolicyRemoteShellCapabilityID]
	if remoteShell == nil {
		t.Fatal("remote-shell capability missing")
	}
	if remoteShell.GetPolicy().GetLocalPolicyRef() != "device-policy/13/remote-shell" {
		t.Fatalf("remote-shell local policy ref = %q", remoteShell.GetPolicy().GetLocalPolicyRef())
	}
	if remoteShell.GetPolicy().GetGrantPolicyRef() != "grant/remote-shell" {
		t.Fatalf("remote-shell grant policy ref = %q", remoteShell.GetPolicy().GetGrantPolicyRef())
	}
	if remoteShell.GetPolicy().GetGrantState() != s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_BLOCKED {
		t.Fatalf("remote-shell grant state = %s", remoteShell.GetPolicy().GetGrantState())
	}
	if remoteShell.GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_GRANT_BLOCKED {
		t.Fatalf("remote-shell state = %s", remoteShell.GetState())
	}
	if remoteShell.GetDetail() != "Space denied terminal" {
		t.Fatalf("remote-shell detail = %q", remoteShell.GetDetail())
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
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true, Detail: "terminal enabled"},
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
	if byID[devicePolicyRemoteShellCapabilityID] == nil {
		t.Fatal("remote-shell capability missing")
	}
}

func TestDevicePolicyEnableShellCommandWritesPolicyAndReloadsDaemon(t *testing.T) {
	// Seed the state path and stub the daemon connection.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)
	statePath := t.TempDir()
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 4}); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	var dialed string
	var reloads int
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		dialed = sockPath
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})

	// Run the enable-shell command and verify the written policy.
	if err := runDeviceCLI(t, "device", "policy", "enable-shell", "--state-path", statePath); err != nil {
		t.Fatalf("device policy enable-shell: %v", err)
	}
	if dialed != filepath.Join(statePath, socketName) {
		t.Fatalf("dialed socket = %q, want state-path socket", dialed)
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if policy.GetRevision() != 5 {
		t.Fatalf("revision = %d, want 5", policy.GetRevision())
	}
	if !policy.GetRemoteShell().GetEnabled() {
		t.Fatal("remote shell was not enabled")
	}
	if policy.GetRemoteShell().GetDetail() != "terminal enabled by local policy" {
		t.Fatalf("remote shell detail = %q", policy.GetRemoteShell().GetDetail())
	}
}

func TestDevicePolicyEnableShellDisableWritesPolicyAndReloadsDaemon(t *testing.T) {
	// Seed the state path and stub the daemon connection.
	clearStatePathEnv(t)
	clearSocketPathEnv(t)
	statePath := t.TempDir()
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{
		Revision:    8,
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true, Detail: "terminal enabled by local policy"},
	}); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	var reloads int
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})

	// Run the enable-shell --disable command and verify the written policy.
	if err := runDeviceCLI(t, "device", "policy", "enable-shell", "--state-path", statePath, "--disable"); err != nil {
		t.Fatalf("device policy enable-shell --disable: %v", err)
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	if policy.GetRevision() != 9 {
		t.Fatalf("revision = %d, want 9", policy.GetRevision())
	}
	if policy.GetRemoteShell().GetEnabled() {
		t.Fatal("remote shell remained enabled")
	}
	if policy.GetRemoteShell().GetDetail() != "terminal disabled by local policy" {
		t.Fatalf("remote shell detail = %q", policy.GetRemoteShell().GetDetail())
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
