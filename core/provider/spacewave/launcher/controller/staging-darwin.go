//go:build darwin

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// getStagingDir returns the platform-specific staging directory for updates.
// On macOS this is ~/Library/Application Support/Spacewave/updates.
func getStagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "get home dir")
	}
	return filepath.Join(home, "Library", "Application Support", "Spacewave", "updates"), nil
}
