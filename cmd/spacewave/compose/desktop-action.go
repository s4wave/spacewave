//go:build !js

package spacewave_compose

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	"github.com/s4wave/spacewave/core/daemon"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/appbundle"
	"github.com/sirupsen/logrus"
)

// openDesktop attaches a no-argument native launch to the shared daemon. The
// connector resolves the state root and starts a verified daemon copy outside
// the app bundle when needed; this process owns only its Resource connection.
func openDesktop(ctx context.Context, _ *logrus.Entry) error {
	return openDesktopWithConnector(ctx, daemon.NewConnector(nil, func(ctx context.Context, statePath string) error {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		return daemon.StartCopiedProcess(ctx, statePath, executable)
	}))
}

// openDesktopWithConnector retains the launcher's connection until the daemon
// acknowledges desktop presence, then releases only that connection.
func openDesktopWithConnector(ctx context.Context, connector *daemon.Connector) error {
	client, err := connector.Connect(ctx, "", "")
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(
		ctx, &desktop_control.OpenOrFocusDesktopRequest{InstalledApp: installedApp()},
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, srpc.ErrUnimplemented) || err.Error() == srpc.ErrUnimplemented.Error() {
		return errors.New("running Spacewave daemon lacks desktop control; upgrade and restart it before opening the desktop")
	}
	return errors.Wrap(err, "open Spacewave desktop")
}

// installedApp returns the application bundle containing this executable, the
// destination for desktop app updates, or empty outside a bundle.
func installedApp() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return ""
	}
	_, appDir := appbundle.Detect(executable)
	return appDir
}
