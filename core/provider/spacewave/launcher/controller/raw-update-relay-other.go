//go:build !js && !windows

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
	if err := syscall.Exec(tmpPath, rawUpdateArgs(tmpPath), rawUpdateRelayEnv(targetPath)); err != nil {
		return errors.Wrap(err, "exec raw update relay")
	}
	return nil
}

func startRawUpdateTarget(targetPath, cleanupPath string) error {
	if err := syscall.Exec(targetPath, rawUpdateArgs(targetPath), rawUpdateTargetEnv(cleanupPath)); err != nil {
		return errors.Wrap(err, "exec raw update target")
	}
	return nil
}

func waitRawUpdateRelayParent() error {
	return nil
}
