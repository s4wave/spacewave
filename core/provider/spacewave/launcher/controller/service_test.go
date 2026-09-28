//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
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

// TestApplyUpdateTargetsInstalledApp verifies only an explicit app request
// hands its verified artifact to Electron; no daemon executable is selected.
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

	// Unknown and daemon targets cannot claim or mutate the staged app.
	for _, target := range []desktop_update.UpdateTarget{
		desktop_update.UpdateTarget_UPDATE_TARGET_UNKNOWN,
		desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON,
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
