package statusprojector

import (
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

func buildUpdateProjection(
	info *spacewave_launcher.LauncherInfo,
) (*desktop_runtime.DesktopRuntimeUpdateStatus, *desktop_runtime.DesktopRuntimeAttentionItem) {
	state := info.GetUpdateState()
	if state == nil {
		return &desktop_runtime.DesktopRuntimeUpdateStatus{}, nil
	}
	status := &desktop_runtime.DesktopRuntimeUpdateStatus{
		Version: state.GetVersion(),
		Label:   updateLabel(state),
		Detail:  updateDetail(state),
	}
	if state.GetPhase() == spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
		status.Ready = true
		return status, &desktop_runtime.DesktopRuntimeAttentionItem{
			Kind:     desktop_runtime.DesktopRuntimeAttentionKind_DESKTOP_RUNTIME_ATTENTION_KIND_UPDATE_READY,
			Severity: desktop_runtime.DesktopRuntimeSeverity_DESKTOP_RUNTIME_SEVERITY_INFO,
			Label:    "Update ready",
			Detail:   updateDetail(state),
		}
	}
	return status, nil
}

func updateLabel(state *spacewave_launcher.UpdateState) string {
	switch state.GetPhase() {
	case spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING:
		return "Downloading update"
	case spacewave_launcher.UpdatePhase_UpdatePhase_STAGED:
		return "Update ready"
	case spacewave_launcher.UpdatePhase_UpdatePhase_APPLYING:
		return "Applying update"
	case spacewave_launcher.UpdatePhase_UpdatePhase_ERROR:
		return "Update failed"
	default:
		return ""
	}
}

func updateDetail(state *spacewave_launcher.UpdateState) string {
	if state.GetErrorMessage() != "" {
		return state.GetErrorMessage()
	}
	if state.GetVersion() != "" {
		return state.GetVersion()
	}
	return ""
}
