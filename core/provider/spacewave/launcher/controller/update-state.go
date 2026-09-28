package spacewave_launcher_controller

import (
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

func (c *Controller) setUpdateError(err error) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		info.UpdateState = &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR,
			ErrorMessage: err.Error(),
			Target:       desktop_update.UpdateTarget_UPDATE_TARGET_APP,
		}
		return true, nil
	})
}

// clearDaemonUpdateState withdraws an obsolete daemon artifact selection.
func (c *Controller) clearDaemonUpdateState() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.DaemonUpdateState == nil {
			return false, nil
		}
		info.DaemonUpdateState = nil
		return true, nil
	})
}

func (c *Controller) clearUpdateState() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.UpdateState == nil {
			return false, nil
		}
		info.UpdateState = nil
		return true, nil
	})
}
