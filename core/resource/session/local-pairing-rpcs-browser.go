//go:build tinygo || goscript

package resource_session

import (
	"context"

	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type localPairingState struct{}

// CreateLocalPairingOffer reports that direct WebRTC local pairing is disabled.
func (r *SessionResource) CreateLocalPairingOffer(ctx context.Context, _ *s4wave_session.CreateLocalPairingOfferRequest) (*s4wave_session.CreateLocalPairingOfferResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("local WebRTC pairing is not available in this browser build")
}

// AcceptLocalPairingOffer reports that direct WebRTC local pairing is disabled.
func (r *SessionResource) AcceptLocalPairingOffer(ctx context.Context, _ *s4wave_session.AcceptLocalPairingOfferRequest) (*s4wave_session.AcceptLocalPairingOfferResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("local WebRTC pairing is not available in this browser build")
}

// AcceptLocalPairingAnswer reports that direct WebRTC local pairing is disabled.
func (r *SessionResource) AcceptLocalPairingAnswer(ctx context.Context, _ *s4wave_session.AcceptLocalPairingAnswerRequest) (*s4wave_session.AcceptLocalPairingAnswerResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("local WebRTC pairing is not available in this browser build")
}
