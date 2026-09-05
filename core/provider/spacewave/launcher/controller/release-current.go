//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
)

// stagedReleaseIsCurrent compares raw installed executables by content rather
// than release labels. Signed app bundles retain their bundle update path.
func (c *Controller) stagedReleaseIsCurrent(stagedPath string) (bool, error) {
	// Only the desktop host can identify the installed entrypoint correctly.
	executable, bundle, _, err := c.currentExecutableBundle()
	if err != nil || bundle {
		return false, err
	}
	installed, err := os.Open(executable)
	if err != nil {
		return false, err
	}
	defer installed.Close()
	staged, err := os.Open(stagedPath)
	if err != nil {
		return false, err
	}
	defer staged.Close()

	// Size mismatches avoid reading complete distribution bundles.
	installedInfo, err := installed.Stat()
	if err != nil {
		return false, err
	}
	stagedInfo, err := staged.Stat()
	if err != nil {
		return false, err
	}
	if !installedInfo.Mode().IsRegular() || !stagedInfo.Mode().IsRegular() || installedInfo.Size() != stagedInfo.Size() {
		return false, nil
	}

	// A version label alone cannot prove that the running bytes are current.
	installedHash, stagedHash := sha256.New(), sha256.New()
	if _, err := io.Copy(installedHash, installed); err != nil {
		return false, err
	}
	if _, err := io.Copy(stagedHash, staged); err != nil {
		return false, err
	}
	return bytes.Equal(installedHash.Sum(nil), stagedHash.Sum(nil)), nil
}
