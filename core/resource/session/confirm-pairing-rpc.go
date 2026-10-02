package resource_session

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ConfirmPairing returns the account attachment persisted by the approved exchange.
// Repeated calls read the same result and cannot grant additional account access.
func (r *SessionResource) ConfirmPairing(ctx context.Context, req *s4wave_session.ConfirmPairingRequest) (*s4wave_session.ConfirmPairingResponse, error) {
	// Refuse confirmation while the Session is locked.
	if r.session.GetPrivKey() == nil {
		return nil, errors.New("session is locked")
	}

	// Open the pairing engine that holds the approved exchange.
	engine, err := r.getPairingEngine()
	if err != nil {
		return nil, err
	}

	// Decode the remote peer ID from the request.
	remotePeerID, err := peer.IDB58Decode(req.GetRemotePeerId())
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}

	// Read the approved pairing result, or an empty response when none is stored.
	ref, err := engine.Result(remotePeerID)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return &s4wave_session.ConfirmPairingResponse{}, nil
	}

	// List registered sessions from the Session controller.
	controller, controllerRef, err := session.ExLookupSessionController(ctx, r.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer controllerRef.Release()
	entries, err := controller.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Return the list entry whose session ref matches the pairing result.
	for _, entry := range entries {
		if entry.GetSessionRef().EqualVT(ref) {
			return &s4wave_session.ConfirmPairingResponse{SessionListEntry: entry}, nil
		}
	}
	return nil, errors.New("paired Session is no longer registered; pair again")
}
