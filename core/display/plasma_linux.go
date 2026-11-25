package display

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/display/gstencoder"
	"github.com/s4wave/spacewave/core/display/portal"
	"github.com/s4wave/spacewave/core/frametrack"
	frametrack_server "github.com/s4wave/spacewave/core/frametrack/server"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// servePlasma owns the portal session for the display track's lifetime.
func servePlasma(ctx context.Context, le *logrus.Entry, b bus.Bus, id protocol.ID) error {
	// The same portal session grants the screen and the input devices.
	session, err := portal.Open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	// Hand the restricted remote to the encoder and release the daemon's FD.
	remote, nodeID, err := session.OpenPipeWireRemote()
	if err != nil {
		return err
	}
	source, err := gstencoder.OpenPipeWireSource(session.Context(), gstencoder.Executable, remote, nodeID)
	_ = remote.Close()
	if err != nil {
		return err
	}

	// Revocation or Session.Closed stops the track and all admitted control streams.
	return frametrack_server.Serve(session.Context(), le, b, id, source, 4, func(*frametrack.Input) {})
}
