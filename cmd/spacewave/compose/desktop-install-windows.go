//go:build windows

package spacewave_compose

import (
	"context"
	"os"
	"time"

	launcher_helper "github.com/s4wave/spacewave/core/launcher/helper"
	launcher_install "github.com/s4wave/spacewave/core/launcher/install"
	"github.com/sirupsen/logrus"
)

// installDesktop makes a downloaded launcher an installed app. When an install
// already exists it starts the installed launcher and reports handedOff.
// Otherwise it installs this executable and returns the loading window that
// shows the install, so the caller keeps it open until the desktop opens. A
// failed install leaves the caller to open the desktop from this executable.
func installDesktop(ctx context.Context, le *logrus.Entry) (window *launcher_helper.Client, handedOff bool) {
	// Only a launcher that embeds its helper installs itself.
	helper := launcher_install.EmbeddedHelper()
	if helper == nil {
		return nil, false
	}
	executable, err := os.Executable()
	if err != nil {
		le.WithError(err).Warn("resolve launcher path")
		return nil, false
	}
	installer, err := launcher_install.NewInstaller()
	if err != nil {
		le.WithError(err).Warn("resolve install path")
		return nil, false
	}
	if installer.Runs(executable) {
		return nil, false
	}

	// Hand a download's launch to the installed launcher, which updates itself.
	if installer.Installed() {
		if err := installer.Start(); err != nil {
			le.WithError(err).Warn("start installed Spacewave")
			return nil, false
		}
		return nil, true
	}

	// Install the helper first so it can show the rest of the install.
	if err := installer.InstallHelper(helper); err != nil {
		le.WithError(err).Warn("install Spacewave helper")
		return nil, false
	}
	window = openHelperWindow(ctx, le, []string{installer.Dir()})
	if window != nil {
		if err := window.SendStatus("Installing Spacewave"); err != nil {
			le.WithError(err).Debug("show install status")
		}
	}

	// Publish the launcher last. Its presence marks a complete install.
	start := time.Now()
	if err := installer.InstallLauncher(executable); err != nil {
		le.WithError(err).Warn("install Spacewave")
		return window, false
	}
	le.WithField("duration", time.Since(start)).Info("installed Spacewave")
	return window, false
}

// installedApp returns the installed launcher, the destination for desktop app
// updates, or an empty string when Spacewave is not installed.
func installedApp() string {
	installer, err := launcher_install.NewInstaller()
	if err != nil || !installer.Installed() {
		return ""
	}
	return installer.Launcher()
}
