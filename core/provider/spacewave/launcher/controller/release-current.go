//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// stagedExecutableSHA256 identifies the exact executable bytes selected for
// an accepted daemon update, independently of its staging pathname.
func stagedExecutableSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// stageDaemonUpdate offers the daemon handoff target when its bytes differ
// from the running daemon. A daemon running from an app bundle copy moves to
// the same executable in the staged app bundle, which carries the desktop
// manifests. Any other daemon moves to the staged CLI. A daemon that cannot
// identify its executable is offered nothing.
func (c *Controller) stageDaemonUpdate(version, appManifestID, stageRoot, distPath, appPath, cliPath string) error {
	executable, bundle, _, err := c.currentExecutableBundle()
	if errors.Is(err, bldr_plugin.ErrHostExecutableUnknown) {
		c.le.WithError(err).Warn("skipping daemon update offer")
		return nil
	}
	if err != nil {
		return err
	}

	// Select the executable matching the running daemon's install shape.
	stagedPath, manifestID := cliPath, cliEntrypointManifestID
	if bundle {
		if !strings.HasSuffix(appPath, ".app") {
			return nil
		}
		stagedPath = filepath.Join(appPath, "Contents", "MacOS", filepath.Base(executable))
		manifestID = appManifestID
		if err := verifyStagedExecutable(stageRoot, distPath, stagedPath); err != nil {
			return errors.Wrap(err, "verify staged app bundle daemon executable")
		}
	}
	if stagedPath == "" {
		return nil
	}

	// Offer the update only when the running bytes differ.
	current, err := sameExecutableBytes(executable, stagedPath)
	if err != nil || current {
		return err
	}
	digest, err := stagedExecutableSHA256(stagedPath)
	if err != nil {
		return errors.Wrap(err, "hash staged daemon executable")
	}
	c.setDaemonUpdateStaged(version, manifestID, stagedPath, digest)
	return nil
}

// sameExecutableBytes compares the running daemon executable to a staged one.
// A version label alone cannot prove that the running bytes are current.
func sameExecutableBytes(runningPath, stagedPath string) (bool, error) {
	running, err := os.Open(runningPath)
	if err != nil {
		return false, err
	}
	defer running.Close()
	staged, err := os.Open(stagedPath)
	if err != nil {
		return false, err
	}
	defer staged.Close()

	// Size mismatches avoid reading complete executables.
	runningInfo, err := running.Stat()
	if err != nil {
		return false, err
	}
	stagedInfo, err := staged.Stat()
	if err != nil {
		return false, err
	}
	if !runningInfo.Mode().IsRegular() || !stagedInfo.Mode().IsRegular() || runningInfo.Size() != stagedInfo.Size() {
		return false, nil
	}

	runningHash, stagedHash := sha256.New(), sha256.New()
	if _, err := io.Copy(runningHash, running); err != nil {
		return false, err
	}
	if _, err := io.Copy(stagedHash, staged); err != nil {
		return false, err
	}
	return bytes.Equal(runningHash.Sum(nil), stagedHash.Sum(nil)), nil
}
