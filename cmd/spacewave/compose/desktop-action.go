//go:build !js

package spacewave_compose

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/daemon"
	"github.com/s4wave/spacewave/core/daemon/desktopcontrol"
	"github.com/sirupsen/logrus"
)

// openDesktop attaches a no-argument native launch to the shared daemon. The
// connector resolves the state root and starts a detached daemon when needed;
// this process owns only its Resource connection through acknowledgement.
func openDesktop(ctx context.Context, _ *logrus.Entry) error {
	return openDesktopWithConnector(ctx, daemon.NewConnector(nil, nil))
}

// openDesktopWithConnector retains the launcher's connection until the daemon
// acknowledges desktop presence, then releases only that connection.
func openDesktopWithConnector(ctx context.Context, connector *daemon.Connector) error {
	client, err := connector.Connect(ctx, "", "")
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = desktopcontrol.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(
		ctx, &desktopcontrol.OpenOrFocusDesktopRequest{},
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, srpc.ErrUnimplemented) || err.Error() == srpc.ErrUnimplemented.Error() {
		return errors.New("running Spacewave daemon lacks desktop control; upgrade and restart it before opening the desktop")
	}
	return errors.Wrap(err, "open Spacewave desktop")
}
