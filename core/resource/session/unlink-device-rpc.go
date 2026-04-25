package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// UnlinkDevice removes a paired device from the account settings SO and
// revokes its SO participant access.
func (r *SessionResource) UnlinkDevice(ctx context.Context, req *s4wave_session.UnlinkDeviceRequest) (*s4wave_session.UnlinkDeviceResponse, error) {
	if r.session.GetPrivKey() == nil {
		return nil, errors.New("session is locked")
	}

	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("unlink device only supported for local provider")
	}

	remotePeerID, err := peer.IDB58Decode(req.GetPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "decode peer ID")
	}

	if remotePeerID == r.session.GetPeerId() {
		return nil, errors.New("cannot unlink own device")
	}

	if err := localAcc.UnlinkDevice(ctx, remotePeerID); err != nil {
		return nil, errors.Wrap(err, "unlink device")
	}

	return &s4wave_session.UnlinkDeviceResponse{}, nil
}
