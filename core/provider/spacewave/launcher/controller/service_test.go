//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

func TestApplyUpdateRecordsFailureInLauncherInfo(t *testing.T) {
	stagedPath := filepath.Join(t.TempDir(), "missing-spacewave")
	ctrl := &Controller{
		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](
			&spacewave_launcher.LauncherInfo{
				UpdateState: &spacewave_launcher.UpdateState{
					Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
					Version:    "0.2.0",
					StagedPath: stagedPath,
				},
			},
		),
	}

	_, err := NewLauncherServer(ctrl).ApplyUpdate(context.Background(), &spacewave_launcher.ApplyUpdateRequest{})
	if err == nil {
		t.Fatal("ApplyUpdate succeeded with missing staged path")
	}
	state := ctrl.launcherInfoCtr.GetValue().GetUpdateState()
	if state.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_ERROR {
		t.Fatalf("phase = %v, want ERROR", state.GetPhase())
	}
	if !strings.Contains(state.GetErrorMessage(), "stat staged path") {
		t.Fatalf("error message = %q, want stat staged path", state.GetErrorMessage())
	}
}
