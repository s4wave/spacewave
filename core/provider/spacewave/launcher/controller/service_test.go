//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	"github.com/s4wave/spacewave/core/daemon"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

func TestApplyUpdateRecordsFailureInLauncherInfo(t *testing.T) {
	stagingDir := t.TempDir()
	stagedPath := filepath.Join(stagingDir, "0.2.0", "dist", "missing-spacewave")
	ctrl := &Controller{
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					Version:    "0.2.0",
					StagedPath: stagedPath,
					Target:     desktop_update.UpdateTarget_UPDATE_TARGET_APP,
				},
			},
		),
	}

	_, err := NewLauncherServer(ctrl).ApplyUpdate(context.Background(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_APP})
	if err == nil {
		t.Fatal("ApplyUpdate succeeded with missing staged path")
	}
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if !strings.Contains(state.GetErrorMessage(), "stat staged") {
		t.Fatalf("error message = %q, want stat staged", state.GetErrorMessage())
	}
}

// TestApplyUpdateTargetsInstalledApp verifies that an explicit app request
// hands its artifact to Electron without accepting the staged daemon update.
func TestApplyUpdateTargetsInstalledApp(t *testing.T) {
	stagingDir := t.TempDir()
	stagedPath := filepath.Join(stagingDir, "0.2.0", "dist", "spacewave")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("desktop artifact"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl := &Controller{
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					Version:    "0.2.0",
					StagedPath: stagedPath,
					Target:     desktop_update.UpdateTarget_UPDATE_TARGET_APP,
				},
				DaemonUpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					StagedPath: filepath.Join(stagingDir, "0.2.0", "cli-dist", "spacewave"),
					Target:     desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
				},
			},
		),
	}
	server := NewLauncherServer(ctrl)

	// An unknown target cannot claim or mutate either staged artifact.
	for _, target := range []desktop_update.UpdateTarget{
		desktop_update.UpdateTarget_UPDATE_TARGET_UNKNOWN,
	} {
		if _, err := server.ApplyUpdate(context.Background(), &desktop_update.ApplyUpdateRequest{Target: target}); err == nil {
			t.Fatalf("target %v was accepted", target)
		}
		if ctrl.launcherInfoCtr.GetValue().GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
			t.Fatalf("target %v changed app state", target)
		}
	}

	// The app target returns the desktop artifact without touching daemon state.
	response, err := server.ApplyUpdate(context.Background(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_APP})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetStagedPath() != stagedPath {
		t.Fatalf("staged path = %q, want %q", response.GetStagedPath(), stagedPath)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatal("app apply changed daemon update state")
	}
}

// TestApplyUpdateAcceptsVerifiedDaemonSelection publishes the selected CLI
// bytes without mutating the separately staged desktop artifact.
func TestApplyUpdateAcceptsVerifiedDaemonSelection(t *testing.T) {
	stagingDir := t.TempDir()
	cliPath := filepath.Join(stagingDir, "0.2.0", "cli-dist", "spacewave")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("selected CLI bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest, err := stagedExecutableSHA256(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := &Controller{
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:  spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					Target: desktop_update.UpdateTarget_UPDATE_TARGET_APP,
				},
				DaemonUpdateState: &spacewave_launcher.UpdateState{
					Phase:              spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					Version:            "0.2.0",
					StagedPath:         cliPath,
					StagedSha256:       digest,
					Target:             desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
					ArtifactManifestId: cliEntrypointManifestID,
				},
				FetchStatus: &spacewave_launcher.FetchStatus{SelectedCliBinaryPath: cliPath},
			},
		),
	}
	server := NewLauncherServer(ctrl)
	releaseWatch := ctrl.attachDaemonUpdateWatcher()
	if _, err := server.ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err != nil {
		t.Fatal(err)
	}
	info := ctrl.launcherInfoCtr.GetValue()
	if info.GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
		t.Fatalf("daemon phase = %v, want APPLYING", info.GetDaemonUpdateState().GetPhase())
	}
	if info.GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		t.Fatal("daemon acceptance changed app update state")
	}
	if _, err := server.ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err != nil {
		t.Fatalf("repeat acceptance changed a pending update: %v", err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
		t.Fatal("repeat acceptance withdrew the pending update")
	}

	// A source changed after acceptance fails preparation while the owner
	// retains the selection for an explicit retry after the bytes are restored.
	selected := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().CloneVT()
	if err := os.WriteFile(cliPath, []byte("changed after acceptance"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.PrepareVerifiedExecutable(t.TempDir(), cliPath, digest); err == nil {
		t.Fatal("changed accepted bytes were prepared")
	}
	failure, err := server.ReportDaemonUpdateFailure(t.Context(), &spacewave_launcher.ReportDaemonUpdateFailureRequest{
		Selection: selected, ErrorMessage: "selected daemon executable changed after acceptance",
	})
	if err != nil || !failure.GetReported() {
		t.Fatalf("report accepted failure: response=%v error=%v", failure, err)
	}
	if state := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState(); state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR || state.GetErrorMessage() == "" {
		t.Fatalf("failed accepted state = %v", state)
	}
	if _, err := server.ReportDaemonUpdateFailure(t.Context(), &spacewave_launcher.ReportDaemonUpdateFailureRequest{
		Selection: selected, ErrorMessage: "stale failure",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("selected CLI bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err != nil {
		t.Fatalf("explicit daemon retry failed: %v", err)
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
		t.Fatal("explicit retry did not republish acceptance")
	}
	releaseWatch()
	if state := ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState(); state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR || !strings.Contains(state.GetErrorMessage(), "owner watch ended") {
		t.Fatalf("lost owner watch left accepted state = %v", state)
	}
	if _, err := server.ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err == nil {
		t.Fatal("daemon update accepted without owner watch")
	}
	releaseRetryWatch := ctrl.attachDaemonUpdateWatcher()
	if _, err := server.ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err != nil {
		t.Fatalf("retry after owner watch returned: %v", err)
	}
	claimed, err := server.ClaimDaemonUpdate(t.Context(), &spacewave_launcher.ClaimDaemonUpdateRequest{
		Selection: ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().CloneVT(),
	})
	if err != nil || !claimed.GetClaimed() {
		t.Fatalf("record owner idle claim: response=%v error=%v", claimed, err)
	}
	releaseRetryWatch()
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
		t.Fatal("completed idle claim was reported as watch failure")
	}
}

// TestDaemonUpdateWaitAndRestartNow publishes the work an accepted update
// waits for and records the user's request to restart without waiting.
func TestDaemonUpdateWaitAndRestartNow(t *testing.T) {
	// Stage a daemon update that the user has not accepted yet.
	selected := &spacewave_launcher.UpdateState{
		Phase:        spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING,
		Target:       desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
		StagedPath:   "fixture-selected-cli",
		StagedSha256: strings.Repeat("a", 64),
	}
	ctrl := &Controller{launcherInfoCtr: ccontainer.NewCContainer(&spacewave_launcher.LauncherInfo{
		DaemonUpdateState: &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
			Target:       selected.GetTarget(),
			StagedPath:   selected.GetStagedPath(),
			StagedSha256: selected.GetStagedSha256(),
		},
	})}
	server := NewLauncherServer(ctrl)

	// Nothing waits before acceptance.
	if _, err := server.RestartDaemonUpdateNow(t.Context(), &spacewave_launcher.RestartDaemonUpdateNowRequest{}); err == nil {
		t.Fatal("restart now succeeded without an accepted update")
	}
	report := &spacewave_launcher.ReportDaemonUpdateWaitRequest{Selection: selected, OtherWork: []string{"gizmo", "spacewave (2)"}}
	if resp, err := server.ReportDaemonUpdateWait(t.Context(), report); err != nil || resp.GetReported() {
		t.Fatalf("wait report before acceptance: response=%v error=%v", resp, err)
	}

	// Accept the staged selection.
	releaseWatch := ctrl.attachDaemonUpdateWatcher()
	defer releaseWatch()
	if err := ctrl.setDaemonUpdateApplying(ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState()); err != nil {
		t.Fatal(err)
	}

	// The accepted selection publishes its waiting work and the restart request.
	if resp, err := server.ReportDaemonUpdateWait(t.Context(), report); err != nil || !resp.GetReported() {
		t.Fatalf("wait report after acceptance: response=%v error=%v", resp, err)
	}
	if _, err := server.RestartDaemonUpdateNow(t.Context(), &spacewave_launcher.RestartDaemonUpdateNowRequest{}); err != nil {
		t.Fatal(err)
	}
	info := ctrl.launcherInfoCtr.GetValue()
	wait := info.GetDaemonUpdateWait()
	if !slices.Equal(wait.GetOtherWork(), report.GetOtherWork()) || !wait.GetRestartNow() {
		t.Fatalf("daemon update wait = %v", wait)
	}
	if !info.GetDaemonUpdateState().EqualVT(selected) {
		t.Fatal("wait report changed the accepted selection")
	}

	// A failure withdraws the wait.
	if !ctrl.setAcceptedDaemonUpdateError(selected, "handoff failed") {
		t.Fatal("accepted failure was not recorded")
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateWait() != nil {
		t.Fatal("failed update kept its wait state")
	}
}

// TestApplyUpdateRejectsChangedDaemonBytes keeps a changed staged executable
// from becoming the daemon's accepted replacement.
func TestApplyUpdateRejectsChangedDaemonBytes(t *testing.T) {
	stagingDir := t.TempDir()
	cliPath := filepath.Join(stagingDir, "0.2.0", "cli-dist", "spacewave")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("changed CLI bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl := &Controller{
		stagingDirFunc: func() (string, error) { return stagingDir, nil },
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				DaemonUpdateState: &spacewave_launcher.UpdateState{
					Phase:              spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED,
					Version:            "0.2.0",
					StagedPath:         cliPath,
					StagedSha256:       strings.Repeat("0", 64),
					Target:             desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
					ArtifactManifestId: cliEntrypointManifestID,
				},
				FetchStatus: &spacewave_launcher.FetchStatus{SelectedCliBinaryPath: cliPath},
			},
		),
	}
	if _, err := NewLauncherServer(ctrl).ApplyUpdate(t.Context(), &desktop_update.ApplyUpdateRequest{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}); err == nil {
		t.Fatal("changed staged daemon bytes were accepted")
	}
	if ctrl.launcherInfoCtr.GetValue().GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR {
		t.Fatal("changed daemon bytes did not publish an error")
	}
}

// TestDaemonOwnerWatchLossPublishesFailure exercises the RPC stream lifetime
// that guards accepted daemon updates when the plugin service disappears.
func TestDaemonOwnerWatchLossPublishesFailure(t *testing.T) {
	initial := &spacewave_launcher.LauncherInfo{DaemonUpdateState: &spacewave_launcher.UpdateState{
		Phase:        spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING,
		Target:       desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
		StagedPath:   "fixture-selected-cli",
		StagedSha256: strings.Repeat("a", 64),
	}}
	ctrl := &Controller{launcherInfoCtr: ccontainer.NewCContainer(initial)}
	mux := srpc.NewMux()
	if err := spacewave_launcher.SRPCRegisterLauncher(mux, NewLauncherServer(ctrl)); err != nil {
		t.Fatal(err)
	}
	client := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	watchCtx, closeWatch := context.WithCancel(ctx)
	stream, err := client.WatchLauncherInfo(watchCtx, &spacewave_launcher.WatchLauncherInfoRequest{DaemonOwner: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatal(err)
	}
	closeWatch()
	_ = stream.Close()
	next, err := ctrl.launcherInfoCtr.WaitValueChange(ctx, initial, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state := next.GetDaemonUpdateState(); state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR || !strings.Contains(state.GetErrorMessage(), "owner watch ended") {
		t.Fatalf("lost owner watch state = %v", state)
	}
}
