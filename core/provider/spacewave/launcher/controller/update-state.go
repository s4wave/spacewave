package spacewave_launcher_controller

import (
	"github.com/pkg/errors"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// setDaemonUpdateApplying publishes an accepted update only if its verified
// selection is still current. The daemon watches this transition independently
// of the request connection.
func (c *Controller) setDaemonUpdateApplying(selected *spacewave_launcher.UpdateState) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.daemonUpdateWatchers == 0 {
		return errors.New("daemon update owner watch is unavailable")
	}
	_, _, err := c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if !info.GetDaemonUpdateState().EqualVT(selected) {
			return false, errors.New("staged daemon update changed before acceptance")
		}
		info.DaemonUpdateState.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING
		info.DaemonUpdateState.ErrorMessage = ""
		info.DaemonUpdateWait = &spacewave_launcher.DaemonUpdateWait{}
		return true, nil
	})
	if err == nil {
		c.daemonUpdateClaimed = false
	}
	return err
}

// hasDaemonUpdateWatcher reports whether acceptance has a live owner stream.
func (c *Controller) hasDaemonUpdateWatcher() bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.daemonUpdateWatchers != 0
}

// attachDaemonUpdateWatcher retains one serving-daemon stream. Losing the last
// stream before the idle claim visibly fails any accepted selection.
func (c *Controller) attachDaemonUpdateWatcher() func() {
	c.mtx.Lock()
	c.daemonUpdateWatchers++
	c.mtx.Unlock()
	return func() {
		c.mtx.Lock()
		defer c.mtx.Unlock()
		c.daemonUpdateWatchers--
		if c.daemonUpdateWatchers != 0 || c.daemonUpdateClaimed {
			return
		}
		_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
			if info.GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
				return false, nil
			}
			info.DaemonUpdateState.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR
			info.DaemonUpdateState.ErrorMessage = "daemon update owner watch ended before idle handoff"
			info.DaemonUpdateWait = nil
			return true, nil
		})
	}
}

// claimAcceptedDaemonUpdate marks only the current accepted selection claimed.
func (c *Controller) claimAcceptedDaemonUpdate(selected *spacewave_launcher.UpdateState) bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.daemonUpdateWatchers == 0 || selected == nil ||
		selected.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING ||
		!c.launcherInfoCtr.GetValue().GetDaemonUpdateState().EqualVT(selected) {
		return false
	}
	c.daemonUpdateClaimed = true
	return true
}

// setDaemonUpdateError reports a failed daemon-target acceptance without
// changing its selection or the separately staged installed-app target.
func (c *Controller) setDaemonUpdateError(err error) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.GetDaemonUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
			return false, nil
		}
		state := info.GetDaemonUpdateState().CloneVT()
		if state == nil {
			state = &spacewave_launcher.UpdateState{Target: desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON}
		}
		state.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR
		state.ErrorMessage = err.Error()
		info.DaemonUpdateState = state
		return true, nil
	})
}

// setAcceptedDaemonUpdateError fails only the accepted selection reported by
// the serving daemon. A later selection cannot be overwritten by an old report.
func (c *Controller) setAcceptedDaemonUpdateError(selected *spacewave_launcher.UpdateState, message string) bool {
	_, changed, _ := c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		current := info.GetDaemonUpdateState()
		if selected == nil || current.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING ||
			current.GetVersion() != selected.GetVersion() ||
			current.GetStagedPath() != selected.GetStagedPath() ||
			current.GetStagedSha256() != selected.GetStagedSha256() ||
			current.GetArtifactManifestId() != selected.GetArtifactManifestId() {
			return false, nil
		}
		info.DaemonUpdateState.Phase = spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR
		info.DaemonUpdateState.ErrorMessage = message
		info.DaemonUpdateWait = nil
		return true, nil
	})
	return changed
}

// setDaemonUpdateWait publishes the other work the accepted selection waits
// for. A report for any other selection changes nothing.
func (c *Controller) setDaemonUpdateWait(selected *spacewave_launcher.UpdateState, clients, services uint32) bool {
	var current bool
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		current = selected.GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING &&
			info.GetDaemonUpdateState().EqualVT(selected)
		if !current {
			return false, nil
		}
		if info.DaemonUpdateWait == nil {
			info.DaemonUpdateWait = &spacewave_launcher.DaemonUpdateWait{}
		}
		info.DaemonUpdateWait.OtherClients = clients
		info.DaemonUpdateWait.OtherServices = services
		return true, nil
	})
	return current
}

// setDaemonUpdateRestartNow asks the serving daemon to claim the accepted
// update without waiting for its other clients and services.
func (c *Controller) setDaemonUpdateRestartNow() error {
	_, _, err := c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.GetDaemonUpdateState().GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
			return false, errors.New("no accepted daemon update is waiting")
		}
		if info.DaemonUpdateWait == nil {
			info.DaemonUpdateWait = &spacewave_launcher.DaemonUpdateWait{}
		}
		info.DaemonUpdateWait.RestartNow = true
		return true, nil
	})
	return err
}

// setUpdateError reports a failed installed-app update.
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

// clearDaemonUpdateState withdraws an obsolete daemon artifact selection unless
// its accepted executable is still waiting for the owner idle handoff.
func (c *Controller) clearDaemonUpdateState() {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.DaemonUpdateState == nil || info.DaemonUpdateState.GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
			return false, nil
		}
		info.DaemonUpdateState = nil
		info.DaemonUpdateWait = nil
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
