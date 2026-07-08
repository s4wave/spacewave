//go:build !js && !goscript && !windows

package spacewave_launcher_controller

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
)

func replaceFile(tmpPath, dstPath string) error {
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return errors.Wrap(err, "replace destination")
	}
	return nil
}

func startRawUpdateRelay(tmpPath, targetPath string) error {
	// #nosec G204 -- the update owner stages tmpPath and targetPath under the
	// verified update roots before replacing the current executable.
	if err := syscall.Exec(tmpPath, rawUpdateArgs(tmpPath), rawUpdateRelayEnv(targetPath)); err != nil {
		return errors.Wrap(err, "exec raw update relay")
	}
	return nil
}

func startRawUpdateTarget(targetPath, cleanupPath string) error {
	// #nosec G204 -- targetPath is the already-installed executable path and
	// cleanupPath is the staged relay file selected by the update owner.
	if err := syscall.Exec(targetPath, rawUpdateArgs(targetPath), rawUpdateTargetEnv(cleanupPath)); err != nil {
		return errors.Wrap(err, "exec raw update target")
	}
	return nil
}

func waitRawUpdateRelayParent() error {
	return nil
}
