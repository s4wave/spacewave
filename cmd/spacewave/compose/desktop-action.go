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
	loader_ui "github.com/s4wave/spacewave/core/provider/spacewave/loader/ui"
	"github.com/sirupsen/logrus"
)

// openDesktop attaches a no-argument native launch to the shared daemon. The
// connector resolves the state root and starts a verified daemon copy outside
// the app bundle when needed. This process owns only its Resource connection.
// A downloaded Windows launcher installs itself first. A failed open stays
// visible in the helper window until a retry succeeds or the person closes
// the window.
func openDesktop(ctx context.Context, le *logrus.Entry) error {
	// Install a downloaded launcher, or hand the launch to the installed one.
	window, handedOff := installDesktop(ctx, le)
	if handedOff {
		return nil
	}
	if window != nil {
		defer closeHelperWindow(le, window)
		if err := window.SendStatus("Opening Spacewave"); err != nil {
			le.WithError(err).Debug("show opening status")
		}
	}

	// Start a verified daemon copy only when no daemon serves the state root.
	connector := daemon.NewConnector(nil, func(ctx context.Context, statePath string) error {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		return daemon.StartCopiedProcess(ctx, statePath, executable)
	})

	// Open the desktop, closing the install window once the desktop opens.
	open := func() error {
		return openDesktopWithConnector(ctx, connector)
	}
	err := open()
	if err == nil {
		dismissHelperWindow(le, window)
		return nil
	}

	// Show the failure in the install window, or in the helper shipped beside
	// this executable.
	if window == nil {
		executable, exeErr := os.Executable()
		if exeErr != nil {
			return err
		}
		window = openHelperWindow(ctx, le, []string{filepath.Dir(executable)})
		if window == nil {
			return err
		}
		defer closeHelperWindow(le, window)
	}
	return showDesktopFailure(ctx, le, window, err, open)
}

// openHelperWindow starts the loading window of the helper found in dirs on a
// pipe in the shared storage root. It returns nil when no helper is found or
// the window cannot start.
func openHelperWindow(ctx context.Context, le *logrus.Entry, dirs []string) *launcher_helper.Client {
	// Locate the helper.
	helperPath, ok := loader_ui.ResolveHelperPathFromDirs(dirs, "", runtime.GOOS)
	if !ok {
		return nil
	}

	// Start the helper window on a pipe in the shared storage root.
	rootDir, err := storagepath.DetermineStorageRoot("spacewave")
	if err != nil {
		le.WithError(err).Warn("resolve helper window root")
		return nil
	}
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		le.WithError(err).Warn("create helper window root")
		return nil
	}
	window, err := launcher_helper.NewLoadingClient(ctx, le, rootDir, helperPath, loader_ui.ResolveIconPathFromDirs(dirs))
	if err != nil {
		le.WithError(err).Warn("helper window unavailable")
		return nil
	}
	return window
}

// dismissHelperWindow dismisses window, if any, once the desktop has opened.
func dismissHelperWindow(le *logrus.Entry, window *launcher_helper.Client) {
	if window == nil {
		return
	}
	if err := window.SendDismiss(); err != nil {
		le.WithError(err).Debug("dismiss helper window")
	}
}

// closeHelperWindow stops the helper behind window.
func closeHelperWindow(le *logrus.Entry, window *launcher_helper.Client) {
	if err := window.Close(); err != nil {
		le.WithError(err).Warn("close helper window")
	}
}

// showDesktopFailure reports err in window and retries the open each time the
// person asks. It returns the latest failure when the window closes.
func showDesktopFailure(
	ctx context.Context,
	le *logrus.Entry,
	window *launcher_helper.Client,
	err error,
	retry func() error,
) error {
	for {
		// Show the failure until the person retries or closes the window.
		le.WithError(err).Warn("desktop did not open")
		if sendErr := window.SendError(desktopFailureMessage(err), true); sendErr != nil {
			return err
		}
		evt, recvErr := window.RecvEvent(ctx)
		if recvErr != nil || evt.GetRetry() == nil {
			return err
		}

		// Dismiss the window once a retry opens the desktop.
		if err = retry(); err == nil {
			dismissHelperWindow(le, window)
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
