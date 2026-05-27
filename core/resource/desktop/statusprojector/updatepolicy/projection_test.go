package updatepolicy

import (
	"testing"

	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

func TestBuildAddsReadyAttention(t *testing.T) {
	status, attention := Build(&spacewave_launcher.LauncherInfo{
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:   spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
			Version: "1.2.3",
		},
	})
	if !status.GetReady() {
		t.Fatalf("ready = false, want true")
	}
	if status.GetLabel() != "Update ready" {
		t.Fatalf("label = %q, want ready label", status.GetLabel())
	}
	if status.GetVersion() != "1.2.3" {
		t.Fatalf("version = %q, want projected version", status.GetVersion())
	}
	if attention == nil {
		t.Fatalf("attention = nil, want update ready")
	}
	if attention.GetKind() != desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_UPDATE_READY {
		t.Fatalf("attention kind = %v, want update ready", attention.GetKind())
	}
}

func TestBuildKeepsDownloadNonReady(t *testing.T) {
	status, attention := Build(&spacewave_launcher.LauncherInfo{
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:   spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING,
			Version: "1.2.3",
		},
	})
	if status.GetReady() {
		t.Fatalf("ready = true, want false")
	}
	if status.GetLabel() != "Downloading update" {
		t.Fatalf("label = %q, want downloading label", status.GetLabel())
	}
	if attention != nil {
		t.Fatalf("attention = %v, want nil before staged", attention)
	}
}

func TestBuildMapsNativeErrors(t *testing.T) {
	status, attention := Build(&spacewave_launcher.LauncherInfo{
		UpdateState: &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
			ErrorMessage: "release metadata missing",
		},
	})
	if status.GetReady() {
		t.Fatalf("ready = true, want false")
	}
	if status.GetLabel() != "Update failed" {
		t.Fatalf("label = %q, want failed label", status.GetLabel())
	}
	if status.GetDetail() != "release metadata missing" {
		t.Fatalf("detail = %q, want launcher error", status.GetDetail())
	}
	if attention != nil {
		t.Fatalf("attention = %v, want nil for non-actionable update error", attention)
	}
}
