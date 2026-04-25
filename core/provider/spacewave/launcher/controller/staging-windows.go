//go:build windows

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// getStagingDir returns the platform-specific staging directory for updates.
// On Windows this is %APPDATA%/Spacewave/updates.
func getStagingDir() (string, error) {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return "", errors.New("APPDATA environment variable not set")
	}
	return filepath.Join(appdata, "Spacewave", "updates"), nil
}
