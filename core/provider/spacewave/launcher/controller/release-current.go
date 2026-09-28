//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
)

// stagedDaemonReleaseIsCurrent compares the staged CLI to the running daemon
// bytes. It says nothing about Electron's separately installed app.
func (c *Controller) stagedDaemonReleaseIsCurrent(stagedPath string) (bool, error) {
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
