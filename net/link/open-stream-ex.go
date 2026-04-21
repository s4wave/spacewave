package link

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// OpenStreamWithPeerEx executes a OpenStreamWithPeer directive.
// Returns a release function for the reference to the link used for the stream.
func OpenStreamWithPeerEx(
	ctx context.Context,
	b bus.Bus,
	protocolID protocol.ID,
	localPeerID, remotePeerID peer.ID,
	transportID uint64,
	openOpts stream.OpenOpts,
) (MountedStream, func(), error) {
	mlnk, _, ref, err := bus.ExecWaitValue[EstablishLinkWithPeerValue](
		ctx,
		b,
		NewEstablishLinkWithPeer(localPeerID, remotePeerID),
		nil,
		nil,
		nil,
	)
	if err != nil {
		if ref != nil {
			ref.Release()
		}
		return nil, func() {}, err
	}

	mstrm, err := mlnk.OpenMountedStream(ctx, protocolID, openOpts)
	if err != nil {
		ref.Release()
		return nil, func() {}, err
	}

	return mstrm, ref.Release, nil
}
