//go:build darwin

package spacewave_launcher_controller

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/pkg/errors"
)

// verifyAppBundleCodesign runs `codesign --verify --deep --strict` against the
// staged .app before the launcher advertises STAGED. A non-zero exit means the
// extracted bundle is tampered, truncated, or unsigned; the caller must wipe
// the staging dir and fall back to DOWNLOADING. The context is passed so an
// updater cancellation aborts the verify.
func verifyAppBundleCodesign(ctx context.Context, appPath string) error {
	cmd := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", appPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "codesign verify failed: %s", stderr.String())
	}
	return nil
}
