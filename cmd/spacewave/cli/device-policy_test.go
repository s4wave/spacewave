//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestDevicePolicyCommandExposesSubcommandsAndFlags(t *testing.T) {
	deviceCmd := newDeviceCommand(nil)
	policyCmd := findTestSubcommand(t, deviceCmd, "policy")
	enableShellCmd := findTestSubcommand(t, policyCmd, "enable-shell")
	checkoutRootCmd := findTestSubcommand(t, policyCmd, "checkout-root")
	checkoutRootAddCmd := findTestSubcommand(t, checkoutRootCmd, "add")
	checkoutRootRemoveCmd := findTestSubcommand(t, checkoutRootCmd, "remove")

	assertCommandFlags(t, enableShellCmd, "state-path", "socket-path", "disable")
	assertCommandFlags(t, checkoutRootAddCmd, "state-path", "socket-path", "write")
	assertCommandFlags(t, checkoutRootRemoveCmd, "state-path", "socket-path")
}

func TestComputeDevicePolicyCapabilitiesProjectsPolicyOwnedCapabilities(t *testing.T) {
	existingLink := &s4wave_device.DeviceCapabilityLink{ObjectKey: "objects/skiffos", TypeId: "unixfs-root"}
	existing := []*s4wave_device.DeviceCapability{
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
		{
			Id:     devicePolicyCheckoutRootIDPrefix + "skiffos",
			Kind:   s4wave_device.DeviceCapabilityKindFilesystem,
			Label:  "old checkout",
			State:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
			Detail: "mounted",
			Link:   existingLink,
			Policy: &s4wave_device.DeviceCapabilityPolicy{
				LocalPolicyRef: "device-policy/12/checkout-root/skiffos",
				GrantPolicyRef: "grant/skiffos",
				LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
				GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
			},
			CheckoutRoot: &s4wave_device.DeviceCheckoutRootCapability{
				Name:          "skiffos",
				DisplayPath:   "/old/skiffos",
				SelectionRef:  "device-policy/12/checkout-root/skiffos",
				Access:        s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
				ReadAvailable: true,
			},
		},
		{
			Id:     devicePolicyCheckoutRootIDPrefix + "removed",
			Kind:   s4wave_device.DeviceCapabilityKindFilesystem,
			Label:  "removed checkout",
			Policy: &s4wave_device.DeviceCapabilityPolicy{LocalPolicyRef: "device-policy/12/checkout-root/removed"},
		},
	}
	policy := &device_policy.DevicePolicy{
		Revision:    13,
		RemoteShell: &device_policy.RemoteShellPolicy{Enabled: true, Detail: "terminal enabled"},
		CheckoutRoot: []*device_policy.CheckoutRootPolicy{
			{
				Name:      "skiffos",
				LocalPath: "/new/skiffos",
				Access:    s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
			},
			{
				Name:      "alpha",
				LocalPath: "/work/alpha",
				Access:    s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY,
			},
		},
	}

	got := computeDevicePolicyCapabilities(policy, existing)
	byID := deviceCapabilitiesByID(got)
	if len(got) != 4 {
		t.Fatalf("capability count = %d, want non-policy + remote shell + two policy roots", len(got))
	}
	if _, ok := byID[devicePolicyCheckoutRootIDPrefix+"removed"]; ok {
		t.Fatal("removed policy checkout root was preserved")
	}
	nonPolicy := byID["custom-capability"]
	if nonPolicy == nil || !nonPolicy.EqualVT(existing[0]) {
		t.Fatalf("non-policy capability = %v, want preserved %v", nonPolicy, existing[0])
	}
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

	skiffos := byID[devicePolicyCheckoutRootIDPrefix+"skiffos"]
	if skiffos == nil {
		t.Fatal("skiffos checkout-root capability missing")
	}
	if skiffos.GetLink().GetObjectKey() != existingLink.GetObjectKey() || skiffos.GetLink().GetTypeId() != existingLink.GetTypeId() {
		t.Fatalf("skiffos link = %v, want preserved %v", skiffos.GetLink(), existingLink)
	}
	if skiffos.GetPolicy().GetLocalPolicyRef() != "device-policy/13/checkout-root/skiffos" {
		t.Fatalf("skiffos local policy ref = %q", skiffos.GetPolicy().GetLocalPolicyRef())
	}
	if skiffos.GetPolicy().GetGrantPolicyRef() != "grant/skiffos" {
		t.Fatalf("skiffos grant policy ref = %q", skiffos.GetPolicy().GetGrantPolicyRef())
	}
	if skiffos.GetPolicy().GetGrantState() != s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED {
		t.Fatalf("skiffos grant state = %s", skiffos.GetPolicy().GetGrantState())
	}
	if skiffos.GetState() != s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE {
		t.Fatalf("skiffos state = %s", skiffos.GetState())
	}
	if skiffos.GetCheckoutRoot().GetDisplayPath() != "/new/skiffos" {
		t.Fatalf("skiffos display path = %q", skiffos.GetCheckoutRoot().GetDisplayPath())
	}
	if skiffos.GetCheckoutRoot().GetSelectionRef() != "device-policy/13/checkout-root/skiffos" {
		t.Fatalf("skiffos selection ref = %q", skiffos.GetCheckoutRoot().GetSelectionRef())
	}
	if skiffos.GetCheckoutRoot().GetAccess() != s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE {
		t.Fatalf("skiffos access = %s", skiffos.GetCheckoutRoot().GetAccess())
	}
	if !skiffos.GetCheckoutRoot().GetReadAvailable() || !skiffos.GetCheckoutRoot().GetWriteAvailable() {
		t.Fatalf("skiffos availability read=%v write=%v", skiffos.GetCheckoutRoot().GetReadAvailable(), skiffos.GetCheckoutRoot().GetWriteAvailable())
	}

	alpha := byID[devicePolicyCheckoutRootIDPrefix+"alpha"]
	if alpha == nil {
		t.Fatal("alpha checkout-root capability missing")
	}
	if alpha.GetPolicy().GetLocalPolicyRef() != "device-policy/13/checkout-root/alpha" {
		t.Fatalf("alpha local policy ref = %q", alpha.GetPolicy().GetLocalPolicyRef())
	}
	if alpha.GetPolicy().GetGrantState() != s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED {
		t.Fatalf("alpha grant state = %s", alpha.GetPolicy().GetGrantState())
	}
	if alpha.GetCheckoutRoot().GetSelectionRef() != "device-policy/13/checkout-root/alpha" {
		t.Fatalf("alpha selection ref = %q", alpha.GetCheckoutRoot().GetSelectionRef())
	}
	if alpha.GetCheckoutRoot().GetAccess() != s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY {
		t.Fatalf("alpha access = %s", alpha.GetCheckoutRoot().GetAccess())
	}
	if !alpha.GetCheckoutRoot().GetReadAvailable() || alpha.GetCheckoutRoot().GetWriteAvailable() {
		t.Fatalf("alpha availability read=%v write=%v", alpha.GetCheckoutRoot().GetReadAvailable(), alpha.GetCheckoutRoot().GetWriteAvailable())
	}
}

func TestProjectDevicePolicyOntoDeviceUpdatesCapabilitiesAndTimestamp(t *testing.T) {
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

	next, changed, err := projectDevicePolicyOntoDevice(existing, policy, now)
	if err != nil {
		t.Fatalf("projectDevicePolicyOntoDevice() error = %v", err)
	}
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
	}, func(_ context.Context, path string) (*exec.Cmd, error) {
		t.Fatal("autostart must not run after successful dial")
		return nil, nil
	})
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})

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
	}, func(_ context.Context, path string) (*exec.Cmd, error) {
		t.Fatal("autostart must not run after successful dial")
		return nil, nil
	})
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})

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

func TestDevicePolicyCheckoutRootAddRemoveWritesPolicyAndReloadsDaemon(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)
	statePath := t.TempDir()
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{Revision: 20}); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	checkoutPath := filepath.Join(t.TempDir(), "skiffos")
	var reloads int
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) (*exec.Cmd, error) {
		t.Fatal("autostart must not run after successful dial")
		return nil, nil
	})
	withDevicePolicyReloadStub(t, func(context.Context, *sdkClient) error {
		reloads++
		return nil
	})

	if err := runDeviceCLI(t, "device", "policy", "checkout-root", "add", "--state-path", statePath, "--write", "skiffos", checkoutPath); err != nil {
		t.Fatalf("device policy checkout-root add: %v", err)
	}
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read add policy: %v", err)
	}
	if policy.GetRevision() != 21 {
		t.Fatalf("add revision = %d, want 21", policy.GetRevision())
	}
	if len(policy.GetCheckoutRoot()) != 1 {
		t.Fatalf("checkout roots after add = %d, want 1", len(policy.GetCheckoutRoot()))
	}
	root := policy.GetCheckoutRoot()[0]
	if root.GetName() != "skiffos" || root.GetLocalPath() != filepath.Clean(checkoutPath) {
		t.Fatalf("checkout root = %v", root)
	}
	if root.GetAccess() != s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE {
		t.Fatalf("checkout root access = %s", root.GetAccess())
	}

	if err := runDeviceCLI(t, "device", "policy", "checkout-root", "remove", "--state-path", statePath, "skiffos"); err != nil {
		t.Fatalf("device policy checkout-root remove: %v", err)
	}
	policy, err = device_policy.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read remove policy: %v", err)
	}
	if policy.GetRevision() != 22 {
		t.Fatalf("remove revision = %d, want 22", policy.GetRevision())
	}
	if len(policy.GetCheckoutRoot()) != 0 {
		t.Fatalf("checkout roots after remove = %d, want 0", len(policy.GetCheckoutRoot()))
	}
	if reloads != 2 {
		t.Fatalf("reloads = %d, want 2", reloads)
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
