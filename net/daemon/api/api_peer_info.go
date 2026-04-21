package bifrost_api

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// GetPeerInfo returns the peer information
func (a *API) GetPeerInfo(
	ctx context.Context,
	req *peer_api.GetPeerInfoRequest,
) (*peer_api.GetPeerInfoResponse, error) {
	var peerID peer.ID
	if peerIDStr := req.GetPeerId(); peerIDStr != "" {
		var err error
		peerID, err = peer.IDB58Decode(peerIDStr)
		if err != nil {
			return nil, errors.Wrap(err, "decode peer id constraint")
		}
	}

	vals, _, ref, err := bus.ExecCollectValues[peer.GetPeerValue](ctx, a.bus, peer.NewGetPeer(peerID), false, nil)
	if err != nil {
		return nil, err
	}
	ref.Release()

	resp := &peer_api.GetPeerInfoResponse{}
	for _, val := range vals {
		resp.LocalPeers = append(resp.LocalPeers, peer_api.NewPeerInfo(val))
	}

	slices.SortFunc(resp.LocalPeers, func(a, b *peer_api.PeerInfo) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	resp.LocalPeers = slices.CompactFunc(resp.LocalPeers, func(a, b *peer_api.PeerInfo) bool {
		return a.GetPeerId() == b.GetPeerId()
	})

	return resp, nil
}
