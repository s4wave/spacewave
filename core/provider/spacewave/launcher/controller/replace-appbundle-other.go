//go:build !darwin && !js

package spacewave_launcher_controller

import "github.com/pkg/errors"

// applyAppBundleUpdate is not supported on non-macOS platforms.
func (c *Controller) applyAppBundleUpdate(currentAppDir, stagedAppDir string) error {
	return errors.New(".app bundle update is only supported on macOS")
}
