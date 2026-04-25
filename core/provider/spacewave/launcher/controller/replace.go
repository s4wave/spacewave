//go:build !js

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// applyUpdate applies the staged update.
// For macOS .app bundles, launches the helper to swap bundles and exits. For
// raw binaries, launches the staged entrypoint as a tmp relay that copies
// itself back to the canonical executable path.
func (c *Controller) applyUpdate() error {
	info := c.launcherInfoCtr.GetValue()
	if info == nil {
		return errors.New("launcher info not available")
	}
	us := info.GetUpdateState()
	if us == nil || us.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
		return errors.New("no staged update available")
	}
	stagedPath := us.GetStagedPath()
	if stagedPath == "" {
		return errors.New("staged path is empty")
	}

	// verify staged path exists
	stagedInfo, err := os.Stat(stagedPath)
	if err != nil {
		return errors.Wrap(err, "stat staged path")
	}

	// get current executable path (resolve symlinks)
	execPath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "get executable path")
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return errors.Wrap(err, "resolve executable symlinks")
	}

	// set applying state
	c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
		li.UpdateState = &spacewave_launcher.UpdateState{
			Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_APPLYING,
			Version:    us.GetVersion(),
			StagedPath: stagedPath,
		}
		return true, nil
	})

	// check if this is a macOS .app bundle update
	isBundle, bundleRoot := detectAppBundle(execPath)
	if isBundle && stagedInfo.IsDir() {
		return c.applyAppBundleUpdate(bundleRoot, stagedPath)
	}

	if stagedInfo.IsDir() {
		return errors.New("staged path is a directory for non-bundle update")
	}

	return applyRawBinaryUpdate(execPath, stagedPath)
}
