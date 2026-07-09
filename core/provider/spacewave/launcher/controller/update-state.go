package spacewave_launcher_controller

import spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"

func (c *Controller) setUpdateError(err error) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		info.UpdateState = &spacewave_launcher.UpdateState{
			Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
			ErrorMessage: err.Error(),
		}
		return true, nil
	})
}

func (c *Controller) clearMatchingUpdateError() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.GetUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_ERROR {
			return false, nil
		}
		info.UpdateState = nil
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
