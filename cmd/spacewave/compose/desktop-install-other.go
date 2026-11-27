//go:build !windows && !js

package spacewave_compose

import (
	"context"
	"os"
	"path/filepath"

	launcher_helper "github.com/s4wave/spacewave/core/launcher/helper"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/appbundle"
	"github.com/sirupsen/logrus"
)

// installDesktop installs nothing. Only the Windows launcher installs itself.
func installDesktop(context.Context, *logrus.Entry) (*launcher_helper.Client, bool) {
	return nil, false
}

// installedApp returns the application bundle containing this executable, the
// destination for desktop app updates, or an empty string outside a bundle.
func installedApp() string {
	// Resolve this process's executable path through its symlinks.
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return ""
	}

	// Detect the enclosing application bundle.
	_, appDir := appbundle.Detect(executable)
	return appDir
}
