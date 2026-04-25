package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ConfirmPairing confirms a verified pairing, adds the remote peer as OWNER
// on all SharedObjects, persists the device, and starts P2P sync.
func (r *SessionResource) ConfirmPairing(ctx context.Context, req *s4wave_session.ConfirmPairingRequest) (*s4wave_session.ConfirmPairingResponse, error) {
	if r.session.GetPrivKey() == nil {
		return nil, errors.New("session is locked")
	}

	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("confirm pairing only supported for local provider")
	}

	remotePeerID, err := peer.IDB58Decode(req.GetRemotePeerId())
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}

	if err := localAcc.ConfirmPairing(ctx, remotePeerID, req.GetDisplayName()); err != nil {
		return nil, errors.Wrap(err, "confirm pairing")
	}

	return &s4wave_session.ConfirmPairingResponse{}, nil
}
