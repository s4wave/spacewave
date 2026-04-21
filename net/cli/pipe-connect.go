package cli

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// runConnect runs the pipe command in connect/client mode.
func (a *PipeArgs) runConnect(ctx context.Context) error {
	// Parse connect address
	remotePeerID, remoteAddr, err := parseConnectAddr(a.ConnectAddr)
	if err != nil {
		return errors.Wrap(err, "parse connect address")
	}

	// Configure dialer for remote peer
	dialers := map[string]*dialer.DialerOpts{
		remotePeerID.String(): {Address: remoteAddr},
	}

	// Setup daemon with UDP transport (ephemeral port)
	d, cleanup, err := a.setupDaemon(ctx, ":0", dialers)
	if err != nil {
		return err
	}
	defer cleanup()

	b := d.GetControllerBus()
	localPeerID := d.GetNodePeerID()

	a.logStatus("Local Peer ID: %s", localPeerID.String())
	a.logStatus("Connecting to %s at %s", remotePeerID.String(), remoteAddr)

	// Open stream to remote peer
	mstrm, rel, err := link.OpenStreamWithPeerEx(
		ctx,
		b,
		protocol.ID(a.ProtocolID),
		localPeerID,
		remotePeerID,
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		return errors.Wrap(err, "open stream")
	}
	defer rel()

	a.logStatus("Connected!")

	// Pipe stream to stdin/stdout
	return pipeStream(mstrm.GetStream(), os.Stdin, os.Stdout)
}
