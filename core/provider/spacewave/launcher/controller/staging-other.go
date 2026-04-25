//go:build !darwin && !windows

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// getStagingDir returns the platform-specific staging directory for updates.
// On Linux this is $XDG_DATA_HOME/spacewave/updates or ~/.local/share/spacewave/updates.
func getStagingDir() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.Wrap(err, "get home dir")
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "spacewave", "updates"), nil
}
