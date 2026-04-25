//go:build darwin

package spacewave_launcher_controller

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
)

// applyAppBundleUpdate extracts the helper from the staged .app, launches it
// in --update mode to swap bundles, then exits the current process so the
// helper can perform the swap and relaunch.
func (c *Controller) applyAppBundleUpdate(currentAppDir, stagedAppDir string) error {
	// Find the helper binary inside the staged .app.
	stagedHelper := filepath.Join(stagedAppDir, "Contents", "MacOS", "spacewave-helper")
	if _, err := os.Stat(stagedHelper); err != nil {
		return errors.Wrap(err, "helper binary not found in staged .app")
	}

	// Copy helper to a persistent location outside both .app bundles so it
	// survives the swap. Use Application Support/bin/.
	stagingDir, err := getStagingDir()
	if err != nil {
		return errors.Wrap(err, "get staging dir")
	}
	helperDir := filepath.Join(filepath.Dir(stagingDir), "bin")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return errors.Wrap(err, "create helper bin dir")
	}

	helperDst := filepath.Join(helperDir, "spacewave-helper")
	if err := copyFileTo(stagedHelper, helperDst); err != nil {
		return errors.Wrap(err, "copy helper to bin dir")
	}
	if err := os.Chmod(helperDst, 0o755); err != nil {
		return errors.Wrap(err, "chmod helper")
	}

	c.le.WithField("helper", helperDst).
		WithField("current", currentAppDir).
		WithField("staged", stagedAppDir).
		Info("launching helper for .app bundle update")

	// Launch the helper in --update mode as a detached subprocess.
	// The helper will:
	// 1. Wait for this process to exit (via PID)
	// 2. Swap the .app bundles (with elevation if needed)
	// 3. Relaunch the new .app via NSWorkspace
	// Pipe args are required by the helper entry point but the update flow
	// gracefully handles pipe connection failure (logs warning, continues).
	pipeRoot := filepath.Dir(stagingDir)
	cmd := exec.Command(helperDst,
		"--update",
		"--current", currentAppDir,
		"--staged", stagedAppDir,
		"--pid", strconv.Itoa(os.Getpid()),
		"--pipe-root", pipeRoot,
		"--pipe-id", "update",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start update helper")
	}

	// Release the child process so it survives our exit.
	if err := cmd.Process.Release(); err != nil {
		c.le.WithError(err).Warn("failed to release helper process")
	}

	c.le.Info("helper launched, exiting for bundle swap")
	os.Exit(0)
	return nil // unreachable
}

// copyFileTo copies a single file from src to dst.
func copyFileTo(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(df, sf); err != nil {
		_ = df.Close()
		return err
	}
	return df.Close()
}
