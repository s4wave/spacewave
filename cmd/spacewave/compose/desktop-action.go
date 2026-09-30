//go:build !js

package spacewave_compose

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	spacewave_cli "github.com/s4wave/spacewave/cmd/spacewave/cli"
	"github.com/s4wave/spacewave/core/daemon"
	launcher_helper "github.com/s4wave/spacewave/core/launcher/helper"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/appbundle"
	loader_ui "github.com/s4wave/spacewave/core/provider/spacewave/loader/ui"
	"github.com/sirupsen/logrus"
)

// openDesktop attaches a no-argument native launch to the shared daemon. The
// connector resolves the state root and starts a verified daemon copy outside
// the app bundle when needed; this process owns only its Resource connection.
// A failed open inside the app bundle stays visible in the helper window until
// a retry succeeds or the person closes the window.
func openDesktop(ctx context.Context, le *logrus.Entry) error {
	// Start a verified daemon copy only when no daemon serves the state root.
	connector := daemon.NewConnector(nil, func(ctx context.Context, statePath string) error {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		return daemon.StartCopiedProcess(ctx, statePath, executable)
	})

	// Open the desktop, and show any failure until a retry succeeds.
	open := func() error {
		return openDesktopWithConnector(ctx, connector)
	}
	err := open()
	if err == nil {
		return nil
	}
	return showDesktopFailure(ctx, le, err, open)
}

// showDesktopFailure reports err in the bundled helper window and retries the
// open each time the person asks. It returns the latest failure when the
// window closes, or err unchanged when no helper ships beside the executable.
func showDesktopFailure(ctx context.Context, le *logrus.Entry, err error, retry func() error) error {
	// Locate the helper shipped beside this executable.
	executable, exeErr := os.Executable()
	if exeErr != nil {
		return err
	}
	dirs := []string{filepath.Dir(executable)}
	helperPath, ok := loader_ui.ResolveHelperPathFromDirs(dirs, "", runtime.GOOS)
	if !ok {
		return err
	}

	// Start the helper window on a pipe in the shared storage root.
	rootDir, rootErr := storagepath.DetermineStorageRoot("spacewave")
	if rootErr != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(rootDir, 0o700); mkdirErr != nil {
		return err
	}
	helper, helperErr := launcher_helper.NewLoadingClient(ctx, le, rootDir, helperPath, loader_ui.ResolveIconPathFromDirs(dirs))
	if helperErr != nil {
		le.WithError(helperErr).Warn("helper window unavailable")
		return err
	}
	defer func() {
		if closeErr := helper.Close(); closeErr != nil {
			le.WithError(closeErr).Warn("close helper window")
		}
	}()

	// Show each failure until the person closes the window or a retry opens the desktop.
	for {
		le.WithError(err).Warn("desktop did not open")
		if sendErr := helper.SendError(desktopFailureMessage(err), true); sendErr != nil {
			return err
		}
		evt, recvErr := helper.RecvEvent(ctx)
		if recvErr != nil || evt.GetRetry() == nil {
			return err
		}
		if err = retry(); err == nil {
			if dismissErr := helper.SendDismiss(); dismissErr != nil {
				le.WithError(dismissErr).Debug("dismiss helper window")
			}
			return nil
		}
	}
}

// desktopFailureMessage returns the one-line reason the helper window shows.
// The log retains the complete error.
func desktopFailureMessage(err error) string {
	if errors.Is(err, errDesktopUIUnavailable) {
		return "Spacewave service has no desktop. Quit it, then retry."
	}
	return "Spacewave could not open. Details are in the log."
}

// errDesktopUIUnavailable marks a daemon that answered with
// spacewave_cli.ErrDesktopUIUnavailable.
var errDesktopUIUnavailable = errors.New("running Spacewave daemon has no desktop UI; quit it, then retry")

// openDesktopWithConnector retains the launcher's connection until the daemon
// acknowledges desktop presence, then releases only that connection.
func openDesktopWithConnector(ctx context.Context, connector *daemon.Connector) error {
	// Connect to the running daemon for the duration of the call.
	client, err := connector.Connect(ctx, "", "")
	if err != nil {
		return err
	}
	defer client.Close()

	// Ask the daemon to open or focus the installed desktop app.
	_, err = desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(
		ctx, &desktop_control.OpenOrFocusDesktopRequest{InstalledApp: installedApp()},
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, srpc.ErrUnimplemented) || err.Error() == srpc.ErrUnimplemented.Error() {
		return errors.New("running Spacewave daemon lacks desktop control; upgrade and restart it before opening the desktop")
	}
	if err.Error() == spacewave_cli.ErrDesktopUIUnavailable.Error() {
		return errDesktopUIUnavailable
	}
	return errors.Wrap(err, "open Spacewave desktop")
}

// installedApp returns the application bundle containing this executable, the
// destination for desktop app updates, or empty outside a bundle.
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
