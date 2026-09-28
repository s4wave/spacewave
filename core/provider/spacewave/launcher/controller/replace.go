//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/appbundle"
)

// prepareAppUpdate returns the verified desktop artifact. Electron owns the
// installed-app destination and exit; this daemon must remain running.
func (c *Controller) prepareAppUpdate(ctx context.Context) (string, error) {
	info := c.launcherInfoCtr.GetValue()
	if info == nil {
		return "", errors.New("launcher info not available")
	}
	state := info.GetUpdateState()
	if state.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_APP || state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED {
		return "", errors.New("no staged app update available")
	}
	stagedPath := state.GetStagedPath()
	if stagedPath == "" {
		return "", errors.New("staged app path is empty")
	}

	// Keep the selected artifact under the launcher's version staging root.
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return "", err
	}
	stageRoot, err := releaseVersionStagingRoot(stagingDir, state.GetVersion())
	if err != nil {
		return "", err
	}
	if err := verifyNoSymlinkPath(stageRoot, stagedPath); err != nil {
		return "", err
	}
	stagedInfo, err := os.Lstat(stagedPath)
	if err != nil {
		return "", errors.Wrap(err, "stat staged app path")
	}
	if stagedInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("staged app path must not be a symlink")
	}

	// Recheck the installed-app shape and signature at the apply boundary.
	if err := c.verifyStagedReleaseEntrypoint(ctx, info.GetFetchStatus().GetSelectedEntrypointPlatformId(), stageRoot, stagedPath); err != nil {
		return "", err
	}
	return stagedPath, nil
}

// prepareDaemonUpdate rechecks the selected CLI artifact before the launcher
// publishes acceptance to the daemon's independent serving lifetime.
func (c *Controller) prepareDaemonUpdate() (*spacewave_launcher.UpdateState, error) {
	// Require the separately selected daemon artifact and its signed manifest.
	info := c.launcherInfoCtr.GetValue()
	if info == nil {
		return nil, errors.New("launcher info not available")
	}
	state := info.GetDaemonUpdateState()
	if state.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON ||
		(state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_STAGED &&
			state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_ERROR) ||
		state.GetArtifactManifestId() != cliEntrypointManifestID ||
		state.GetStagedPath() == "" ||
		state.GetStagedPath() != info.GetFetchStatus().GetSelectedCliBinaryPath() {
		return nil, errors.New("no staged daemon update available")
	}

	// Recheck the selected path inside its version-specific CLI checkout.
	stagingDir, err := c.resolveStagingDir()
	if err != nil {
		return nil, err
	}
	stageRoot, err := releaseVersionStagingRoot(stagingDir, state.GetVersion())
	if err != nil {
		return nil, err
	}
	cliDistPath := filepath.Join(stageRoot, "cli-dist")
	if err := verifyStagedCLIEntrypoint(stageRoot, cliDistPath, state.GetStagedPath()); err != nil {
		return nil, err
	}
	digest, err := stagedExecutableSHA256(state.GetStagedPath())
	if err != nil {
		return nil, errors.Wrap(err, "hash staged daemon executable")
	}
	if digest == "" || digest != state.GetStagedSha256() {
		return nil, errors.New("staged daemon executable fails selected digest")
	}
	return state.CloneVT(), nil
}

// currentExecutableBundle resolves the daemon executable for daemon-specific
// comparisons and diagnostics, never as the installed-app update destination.
func (c *Controller) currentExecutableBundle() (string, bool, string, error) {
	if c.currentExecutableBundleFunc != nil {
		return c.currentExecutableBundleFunc()
	}
	execPath, err := os.Executable()
	if err != nil {
		return "", false, "", errors.Wrap(err, "get executable path")
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", false, "", errors.Wrap(err, "resolve executable symlinks")
	}
	isBundle, bundleRoot := appbundle.Detect(execPath)
	return execPath, isBundle, bundleRoot, nil
}
