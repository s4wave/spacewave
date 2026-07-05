//go:build !js

package spacewave_cli

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	core_provider "github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestDeviceCommandExposesDaemonFlags(t *testing.T) {
	assertCommandFlags(t, newDeviceCommand(nil), "state-path", "socket-path")
	assertCommandFlags(t, newDeviceSetupCommand(), "state-path", "socket-path", "label", "target-hint", "role", "expires-in", "output")
	assertCommandFlags(t, newDeviceSetupDockerCommand(), "state-path", "socket-path", "label", "output")
	assertCommandFlags(t, newDeviceCompleteCommand(), "state-path", "socket-path", "completion", "output")
	assertCommandFlags(t, newDeviceStatusCommand(), "state-path", "socket-path", "output")
}

func TestDeviceCommandRegistered(t *testing.T) {
	for _, cmd := range NewCliCommands(nil) {
		if cmd.Name == "device" {
			return
		}
	}
	t.Fatal("device command not registered")
}

func TestDeviceSetupDockerUsesGroupStatePath(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "device-state")
	out, err := captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"--state-path",
			statePath,
			"setup",
			"docker",
			"--label",
			"build-host",
		)
	})
	if err != nil {
		t.Fatalf("device setup docker: %v", err)
	}
	assertContains(t, out, "Label")
	assertContains(t, out, "build-host")
	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, filepath.Join(statePath, socketName))
	assertContains(t, out, deviceDockerStatePath)
	assertContains(t, out, "SESSION_TYPE_DEVICE")
	assertContains(t, out, "WRITER")
	assertContains(t, out, "cli-mediated")
	assertContains(t, out, "not started")
	assertContains(t, out, "not generated")
}

func TestDeviceSetupDockerAcceptsLeafSocketFlag(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "device-state")
	socketPath := filepath.Join(t.TempDir(), "device.sock")
	out, err := captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"setup",
			"docker",
			"--state-path",
			statePath,
			"--socket-path",
			socketPath,
			"--label",
			"build-host",
		)
	})
	if err != nil {
		t.Fatalf("device setup docker: %v", err)
	}
	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, "Socket")
	assertContains(t, out, socketPath)
}

func TestDeviceSetupUsesResolvedStatePathAndAutostart(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	var startedStatePath string
	var dialed []string
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		dialed = append(dialed, sockPath)
		if call == 1 {
			return nil, os.ErrNotExist
		}
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		startedStatePath = path
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath)
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	if startedStatePath != statePath {
		t.Fatalf("started state path = %q, want %q", startedStatePath, statePath)
	}
	wantSock := filepath.Join(statePath, socketName)
	if strings.Join(dialed, ",") != wantSock+","+wantSock {
		t.Fatalf("dialed sockets = %v, want two dials of %s", dialed, wantSock)
	}
	if !strings.Contains(out, "Setup:") || !strings.Contains(out, "waiting for completion") {
		t.Fatalf("setup output missing waiting state: %s", out)
	}

	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.SetupState != deviceSetupStateWaiting {
		t.Fatalf("setup state = %q, want %q", record.SetupState, deviceSetupStateWaiting)
	}
}

func TestDeviceSetupUsesGlobalStatePathFlag(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "global")
	var dialed string
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		dialed = sockPath
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLIWithGlobalStatePath(t, statePath, "device", "setup", "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	if dialed != filepath.Join(statePath, socketName) {
		t.Fatalf("dialed socket = %q, want state-path socket", dialed)
	}
	var got deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &got); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, out)
	}
	if got.StatePath != statePath {
		t.Fatalf("state path = %q, want %q", got.StatePath, statePath)
	}
	if _, err := readDeviceSetupRecord(statePath); err != nil {
		t.Fatalf("read global setup record: %v", err)
	}
}

func TestDeviceSetupUsesEnvStatePath(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "env")
	if err := os.Setenv(statePathEnvVars[0], statePath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(statePathEnvVars[0])
	})
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var got deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &got); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, out)
	}
	if got.StatePath != statePath {
		t.Fatalf("state path = %q, want %q", got.StatePath, statePath)
	}
	if _, err := readDeviceSetupRecord(statePath); err != nil {
		t.Fatalf("read env setup record: %v", err)
	}
}

func TestDeviceSetupOutputsSignedDeviceTicketAndReusesIdentity(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"setup",
			"--state-path",
			statePath,
			"--label",
			"build host",
			"--target-hint",
			"space-1",
			"--role",
			"writer",
			"--expires-in",
			"10m",
			"--output",
			"json",
		)
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var first deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &first); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, out)
	}
	if !first.IdentityCreated {
		t.Fatal("first setup did not report identity creation")
	}
	assertDeviceTicket(t, first.Ticket, first.PeerID, "build host", "space-1", sobject.SOParticipantRole_SOParticipantRole_WRITER)

	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.Ticket != first.Ticket {
		t.Fatal("setup record did not persist ticket")
	}
	if record.PeerID != first.PeerID {
		t.Fatal("setup record peer id mismatch")
	}

	out, err = captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"setup",
			"--state-path",
			statePath,
			"--label",
			"build host",
			"--output",
			"json",
		)
	})
	if err != nil {
		t.Fatalf("second device setup: %v", err)
	}
	var second deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &second); err != nil {
		t.Fatalf("parse second setup json: %v: %s", err, out)
	}
	if second.IdentityCreated {
		t.Fatal("second setup should reuse the existing identity")
	}
	if second.PeerID != first.PeerID {
		t.Fatalf("second peer id = %q, want %q", second.PeerID, first.PeerID)
	}
}

func TestDeviceCompleteImportsApprovalCompletionIntoSetupState(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	setupOut, err := captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"setup",
			"--state-path",
			statePath,
			"--label",
			"build host",
			"--output",
			"json",
		)
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var setup deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(setupOut), &setup); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, setupOut)
	}
	var mountReq *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest
	var upsertRecord *deviceSetupRecord
	withDeviceMountSessionStub(t, func(
		ctx context.Context,
		client *sdkClient,
		req *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest,
	) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error) {
		mountReq = req.CloneVT()
		return &s4wave_provider_spacewave.MountLinkedDeviceSessionResponse{
			SessionListEntry: &core_session.SessionListEntry{
				SessionIndex: 7,
				SessionRef: &core_session.SessionRef{
					ProviderResourceRef: &core_provider.ProviderResourceRef{
						Id:                req.GetSessionId(),
						ProviderAccountId: req.GetAccountId(),
						ProviderId:        "spacewave",
					},
				},
			},
		}, nil
	})
	withDeviceObjectUpsertStub(t, func(ctx context.Context, client *sdkClient, record *deviceSetupRecord) (string, error) {
		upsertRecord = record
		return "devices/build-host", nil
	})

	completion := buildDeviceCompletion(
		t,
		setup.Ticket,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK,
		"",
	)
	out, err := captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"complete",
			"--state-path",
			statePath,
			"--completion",
			completion,
			"--output",
			"json",
		)
	})
	if err != nil {
		t.Fatalf("device complete: %v", err)
	}
	var got deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &got); err != nil {
		t.Fatalf("parse complete json: %v: %s", err, out)
	}
	if got.SetupState != deviceSetupStateSessionReady {
		t.Fatalf("setup state = %q, want %q", got.SetupState, deviceSetupStateSessionReady)
	}
	if got.CompletionStatus != "ok" {
		t.Fatalf("completion status = %q, want ok", got.CompletionStatus)
	}
	if got.AccountID != "acct-device" {
		t.Fatalf("account id = %q, want acct-device", got.AccountID)
	}
	if got.ResourceID != base64.StdEncoding.EncodeToString([]byte("space-1")) {
		t.Fatalf("resource id = %q", got.ResourceID)
	}
	if got.SessionPeerID != setup.PeerID {
		t.Fatalf("session peer = %q, want %q", got.SessionPeerID, setup.PeerID)
	}
	if got.SessionID == "" {
		t.Fatal("session id is empty")
	}
	if got.SessionIndex != 7 {
		t.Fatalf("session index = %d, want 7", got.SessionIndex)
	}
	if got.DeviceObjectKey != "devices/build-host" {
		t.Fatalf("device object key = %q, want devices/build-host", got.DeviceObjectKey)
	}
	if mountReq == nil {
		t.Fatal("device completion did not mount linked session")
	}
	if mountReq.GetAccountId() != "acct-device" {
		t.Fatalf("mount account id = %q, want acct-device", mountReq.GetAccountId())
	}
	if mountReq.GetSessionId() != got.SessionID {
		t.Fatalf("mount session id = %q, want %q", mountReq.GetSessionId(), got.SessionID)
	}
	if mountReq.GetLabel() != "build host" {
		t.Fatalf("mount label = %q, want build host", mountReq.GetLabel())
	}
	if mountReq.GetSessionPeerId() != setup.PeerID {
		t.Fatalf("mount peer id = %q, want %q", mountReq.GetSessionPeerId(), setup.PeerID)
	}
	if len(mountReq.GetSessionPemPrivateKey()) == 0 {
		t.Fatal("mount request missing session PEM")
	}
	if upsertRecord == nil {
		t.Fatal("device completion did not create or update the Device object")
	}
	if upsertRecord.SessionIndex != 7 {
		t.Fatalf("upsert session index = %d, want 7", upsertRecord.SessionIndex)
	}
	if upsertRecord.ResourceID != base64.StdEncoding.EncodeToString([]byte("space-1")) {
		t.Fatalf("upsert resource id = %q", upsertRecord.ResourceID)
	}

	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.SetupState != deviceSetupStateSessionReady || record.Completion != completion {
		t.Fatalf("record did not persist imported completion: %#v", record)
	}
	if record.DeviceObjectKey != "devices/build-host" {
		t.Fatalf("record device object key = %q, want devices/build-host", record.DeviceObjectKey)
	}
}

func TestDeviceCompletePersistsCompletionWhenSessionMountFails(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	setupOut, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var setup deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(setupOut), &setup); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, setupOut)
	}
	withDeviceMountSessionStub(t, func(
		ctx context.Context,
		client *sdkClient,
		req *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest,
	) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error) {
		return nil, errors.New("session unavailable")
	})

	completion := buildDeviceCompletion(
		t,
		setup.Ticket,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK,
		"",
	)
	_, err = captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "complete", "--state-path", statePath, "--completion", completion)
	})
	if err == nil {
		t.Fatal("device complete succeeded despite mount failure")
	}
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.SetupState != deviceSetupStateImported {
		t.Fatalf("setup state = %q, want imported completion after mount failure", record.SetupState)
	}
	if record.Completion != completion {
		t.Fatal("completion payload was not preserved after mount failure")
	}
}

func TestDeviceCompletePreservesCompletionWhenDeviceObjectUpsertFails(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	setupOut, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var setup deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(setupOut), &setup); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, setupOut)
	}
	withDeviceMountSessionStub(t, func(
		ctx context.Context,
		client *sdkClient,
		req *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest,
	) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error) {
		return &s4wave_provider_spacewave.MountLinkedDeviceSessionResponse{
			SessionListEntry: &core_session.SessionListEntry{SessionIndex: 9},
		}, nil
	})
	withDeviceObjectUpsertStub(t, func(ctx context.Context, client *sdkClient, record *deviceSetupRecord) (string, error) {
		return "", errors.New("world write rejected")
	})

	completion := buildDeviceCompletion(
		t,
		setup.Ticket,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK,
		"",
	)
	_, err = captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "complete", "--state-path", statePath, "--completion", completion)
	})
	if err == nil {
		t.Fatal("device complete succeeded despite Device object write failure")
	}
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.SetupState != deviceSetupStateImported {
		t.Fatalf("setup state = %q, want imported completion after object write failure", record.SetupState)
	}
	if record.Completion != completion {
		t.Fatal("completion payload was not preserved after object write failure")
	}
	if record.DeviceObjectKey != "" {
		t.Fatalf("device object key = %q, want empty after failed write", record.DeviceObjectKey)
	}
}

func TestDeviceCompleteRecordsRetryableFailureAndSetupCanRegenerateTicket(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	setupOut, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var setup deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(setupOut), &setup); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, setupOut)
	}

	completion := buildDeviceCompletion(
		t,
		setup.Ticket,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_DENIED,
		"owner denied approval",
	)
	out, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "complete", "--state-path", statePath, "--completion", completion, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device complete denied: %v", err)
	}
	var failed deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &failed); err != nil {
		t.Fatalf("parse failed completion json: %v: %s", err, out)
	}
	if failed.SetupState != deviceSetupStateFailed {
		t.Fatalf("setup state = %q, want %q", failed.SetupState, deviceSetupStateFailed)
	}
	if failed.CompletionStatus != "denied" || failed.FailureReason != "owner denied approval" {
		t.Fatalf("failure = %q/%q, want denied/owner denied approval", failed.CompletionStatus, failed.FailureReason)
	}

	retryOut, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("retry setup: %v", err)
	}
	var retry deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(retryOut), &retry); err != nil {
		t.Fatalf("parse retry setup json: %v: %s", err, retryOut)
	}
	if retry.SetupState != deviceSetupStateWaiting {
		t.Fatalf("retry setup state = %q, want %q", retry.SetupState, deviceSetupStateWaiting)
	}
	if retry.IdentityCreated {
		t.Fatal("retry setup should reuse the existing device identity")
	}
	if retry.PeerID != setup.PeerID {
		t.Fatalf("retry peer id = %q, want %q", retry.PeerID, setup.PeerID)
	}
	if retry.Ticket == setup.Ticket {
		t.Fatal("retry setup reused the denied ticket")
	}
}

func TestDeviceCompleteRejectsMismatchedNonceWithoutChangingState(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	setupOut, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "setup", "--state-path", statePath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device setup: %v", err)
	}
	var setup deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(setupOut), &setup); err != nil {
		t.Fatalf("parse setup json: %v: %s", err, setupOut)
	}
	completion := buildDeviceCompletion(
		t,
		setup.Ticket,
		s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK,
		"",
	)
	completionBytes, err := base64.StdEncoding.DecodeString(completion)
	if err != nil {
		t.Fatalf("decode completion: %v", err)
	}
	callback := &s4wave_provider_spacewave.SpaceLinkCallback{}
	if err := callback.UnmarshalVT(completionBytes); err != nil {
		t.Fatalf("unmarshal completion: %v", err)
	}
	callback.Nonce = []byte("not-the-ticket-nonce")
	mutated, err := callback.MarshalVT()
	if err != nil {
		t.Fatalf("marshal mutated completion: %v", err)
	}

	_, err = captureStdout(t, func() error {
		return runDeviceCLI(
			t,
			"device",
			"complete",
			"--state-path",
			statePath,
			"--completion",
			base64.StdEncoding.EncodeToString(mutated),
		)
	})
	if err == nil {
		t.Fatal("device complete accepted mismatched nonce")
	}
	record, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatalf("read setup record: %v", err)
	}
	if record.SetupState != deviceSetupStateWaiting {
		t.Fatalf("setup state changed after rejected completion: %q", record.SetupState)
	}
}

func TestDeviceStatusUsesSocketOverrideWithoutAutostart(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	socketPath := filepath.Join(t.TempDir(), "desktop.sock")
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{SetupState: deviceSetupStateLocalReady}); err != nil {
		t.Fatalf("write setup record: %v", err)
	}
	var dialed string
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		dialed = sockPath
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run with --socket-path")
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "status", "--state-path", statePath, "--socket-path", socketPath, "--output", "json")
	})
	if err != nil {
		t.Fatalf("device status: %v", err)
	}
	if dialed != socketPath {
		t.Fatalf("dialed socket = %q, want %q", dialed, socketPath)
	}
	var got deviceStatusOutput
	if err := parseDeviceStatusOutputJSON([]byte(out), &got); err != nil {
		t.Fatalf("parse status json: %v: %s", err, out)
	}
	if got.SetupState != deviceSetupStateLocalReady {
		t.Fatalf("setup state = %q, want %q", got.SetupState, deviceSetupStateLocalReady)
	}
	if got.Socket != socketPath {
		t.Fatalf("socket = %q, want %q", got.Socket, socketPath)
	}
	if got.StatePath != statePath {
		t.Fatalf("state path = %q, want %q", got.StatePath, statePath)
	}
}

func TestDeviceStatusReportsNotConfiguredWhenSetupStateMissing(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "state")
	withDeviceDaemonStub(t, func(sockPath string, call int) (net.Conn, error) {
		return newTestDaemonConn(t), nil
	}, func(_ context.Context, path string) error {
		t.Fatal("autostart must not run after successful dial")
		return nil
	})

	out, err := captureStdout(t, func() error {
		return runDeviceCLI(t, "device", "status", "--state-path", statePath)
	})
	if err != nil {
		t.Fatalf("device status: %v", err)
	}
	if !strings.Contains(out, "Setup:") || !strings.Contains(out, "not configured") {
		t.Fatalf("status output missing not configured: %s", out)
	}
}

func runDeviceCLI(t *testing.T, args ...string) error {
	t.Helper()

	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Commands = []*cli.Command{newDeviceCommand(nil)}
	return app.RunContext(context.Background(), append([]string{"spacewave"}, args...))
}

func runDeviceCLIWithGlobalStatePath(t *testing.T, statePath string, args ...string) error {
	t.Helper()

	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{statePathFlag(&rootStatePath)}
	app.Commands = []*cli.Command{newDeviceCommand(nil)}
	return app.RunContext(context.Background(), append([]string{"spacewave", "--state-path", statePath}, args...))
}

func buildDeviceCompletion(
	t *testing.T,
	ticket string,
	status s4wave_provider_spacewave.SpaceLinkCallbackStatus,
	message string,
) string {
	t.Helper()

	payload, err := decodeDeviceStoredTicketPayload(&deviceSetupRecord{Ticket: ticket})
	if err != nil {
		t.Fatalf("decode setup ticket payload: %v", err)
	}
	completion := &s4wave_provider_spacewave.SpaceLinkCallback{
		Status:       status,
		Nonce:        payload.GetNonce(),
		ErrorMessage: message,
	}
	if status == s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK {
		completion.AccountId = "acct-device"
		completion.ResourceId = []byte("space-1")
		completion.SessionPeerId = payload.GetAgentPeerId()
	}
	data, err := completion.MarshalVT()
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func withDeviceMountSessionStub(
	t *testing.T,
	mount func(context.Context, *sdkClient, *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error),
) {
	t.Helper()

	oldMount := deviceMountLinkedSession
	t.Cleanup(func() {
		deviceMountLinkedSession = oldMount
	})
	deviceMountLinkedSession = mount
}

func withDeviceObjectUpsertStub(
	t *testing.T,
	upsert func(context.Context, *sdkClient, *deviceSetupRecord) (string, error),
) {
	t.Helper()

	oldUpsert := deviceUpsertObject
	t.Cleanup(func() {
		deviceUpsertObject = oldUpsert
	})
	deviceUpsertObject = upsert
}

func assertDeviceTicket(
	t *testing.T,
	encoded string,
	wantPeerID string,
	wantLabel string,
	wantTargetHint string,
	wantRole sobject.SOParticipantRole,
) {
	t.Helper()

	ticketBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{}
	if err := ticket.UnmarshalVT(ticketBytes); err != nil {
		t.Fatalf("unmarshal ticket: %v", err)
	}
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{}
	if err := payload.UnmarshalVT(ticket.GetPayload()); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	agentPeerID, err := peer.IDFromBytes(payload.GetAgentPeerId())
	if err != nil {
		t.Fatalf("parse peer id: %v", err)
	}
	if agentPeerID.String() != wantPeerID {
		t.Fatalf("ticket peer id = %q, want %q", agentPeerID.String(), wantPeerID)
	}
	pub, err := agentPeerID.ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract public key: %v", err)
	}
	ok, err := pub.Verify(ticket.GetPayload(), ticket.GetAgentSignature())
	if err != nil {
		t.Fatalf("verify signature: %v", err)
	}
	if !ok {
		t.Fatal("ticket signature did not verify")
	}
	if payload.GetVersion() != deviceSpaceLinkAuthRequestVersion {
		t.Fatalf("ticket version = %d, want %d", payload.GetVersion(), deviceSpaceLinkAuthRequestVersion)
	}
	if payload.GetSessionType() != core_session.SessionType_SESSION_TYPE_DEVICE {
		t.Fatalf("session type = %v, want DEVICE", payload.GetSessionType())
	}
	if payload.GetLabel() != wantLabel {
		t.Fatalf("label = %q, want %q", payload.GetLabel(), wantLabel)
	}
	if string(payload.GetTargetHint()) != wantTargetHint {
		t.Fatalf("target hint = %q, want %q", string(payload.GetTargetHint()), wantTargetHint)
	}
	if payload.GetRequestedRole() != wantRole {
		t.Fatalf("role = %v, want %v", payload.GetRequestedRole(), wantRole)
	}
	if len(payload.GetNonce()) != deviceSetupNonceLength {
		t.Fatalf("nonce length = %d, want %d", len(payload.GetNonce()), deviceSetupNonceLength)
	}
	if payload.GetCompletionMode() != s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI {
		t.Fatalf("completion mode = %v, want CLI", payload.GetCompletionMode())
	}
	if payload.GetCallbackUrl() != "" {
		t.Fatalf("callback url = %q, want empty", payload.GetCallbackUrl())
	}
	if time.Unix(payload.GetExpiresAt(), 0).Before(time.Now()) {
		t.Fatalf("ticket already expired at %d", payload.GetExpiresAt())
	}
}

func parseDeviceStatusOutputJSON(data []byte, out *deviceStatusOutput) error {
	var parser fastjson.Parser
	v, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if v.Type() != fastjson.TypeObject {
		return errors.New("device status output must be object")
	}
	out.DaemonStatus = string(v.GetStringBytes("daemonStatus"))
	out.SetupState = string(v.GetStringBytes("setupState"))
	out.StatePath = string(v.GetStringBytes("statePath"))
	out.Socket = string(v.GetStringBytes("socket"))
	out.PeerID = string(v.GetStringBytes("peerId"))
	out.Label = string(v.GetStringBytes("label"))
	out.RequestedRole = string(v.GetStringBytes("requestedRole"))
	out.TargetHint = string(v.GetStringBytes("targetHint"))
	out.CompletionMode = string(v.GetStringBytes("completionMode"))
	out.CompletionAt = v.GetInt64("completionAt")
	out.CompletionStatus = string(v.GetStringBytes("completionStatus"))
	out.AccountID = string(v.GetStringBytes("accountId"))
	out.ResourceID = string(v.GetStringBytes("resourceId"))
	out.SessionID = string(v.GetStringBytes("sessionId"))
	out.SessionIndex = uint32(v.GetUint("sessionIndex"))
	out.SessionPeerID = string(v.GetStringBytes("sessionPeerId"))
	out.DeviceObjectKey = string(v.GetStringBytes("deviceObjectKey"))
	out.FailureReason = string(v.GetStringBytes("failureReason"))
	out.ExpiresAt = v.GetInt64("expiresAt")
	out.Ticket = string(v.GetStringBytes("ticket"))
	out.IdentityCreated = v.GetBool("identityCreated")
	return nil
}

func withDeviceDaemonStub(
	t *testing.T,
	dial func(sockPath string, call int) (net.Conn, error),
	start func(context.Context, string) error,
) {
	t.Helper()

	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	var dialCount int
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCount++
		return dial(sockPath, dialCount)
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}
	connectDaemonStart = start
}

func newTestDaemonConn(t *testing.T) net.Conn {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})
	return clientConn
}
