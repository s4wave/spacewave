package link

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/peer"
)

// EstablishLinkWithPeerEx executes a EstablishLinkWithPeer directive.
// Returns a release function.
// if returnIfIdle: returns nil, nil, nil if not found (idle directive)
func EstablishLinkWithPeerEx(
	ctx context.Context,
	b bus.Bus,
	localPeerID, remotePeerID peer.ID,
	returnIfIdle bool,
) (EstablishLinkWithPeerValue, func(), error) {
	estl, _, ref, err := bus.ExecWaitValue[EstablishLinkWithPeerValue](
		ctx,
		b,
		NewEstablishLinkWithPeer(localPeerID, remotePeerID),
		bus.ReturnIfIdle(returnIfIdle),
		nil,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if estl == nil {
		ref.Release()
		return nil, nil, nil
	}

	return estl, ref.Release, nil
}
