package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/transport"
)

// ErrNoSessionTransport indicates an unavailable account transport.
var ErrNoSessionTransport = errors.New("no session transport running")

// GetPairingEngine returns this unlocked Session's account enrollment owner.
func (s *Session) GetPairingEngine() (*pairing.Engine, error) {
	// Return the existing engine, or build one on the Session's lifetime.
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	if s.GetPrivKey() == nil || s.ctx.Err() != nil {
		return nil, errors.New("Session is locked or closed")
	}
	if s.pairingEngine != nil {
		return s.pairingEngine, nil
	}

	// Build and retain the pairing engine for this Session.
	ctx, cancel := context.WithCancel(s.ctx)
	a := s.tkr.a
	engine, err := pairing.NewEngine(ctx, a.le, a.t.p.b, s, a, s.GetPairingTransport)
	if err != nil {
		cancel()
		return nil, err
	}
	s.pairingEngine, s.pairingCancel = engine, cancel
	return engine, nil
}

func (s *Session) clearPairingEngine() {
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	if s.pairingCancel != nil {
		s.pairingCancel()
	}
	s.pairingEngine, s.pairingCancel = nil, nil
}

// GetPairingTransport uses this Session's existing connection owner. An empty
// relay requests a direct connection and does not replace working signaling.
func (s *Session) GetPairingTransport(ctx context.Context, relay pairing.Relay) (*transport.SessionTransport, error) {
	// Reuse the account transport when it already serves this Session.
	a := s.tkr.a
	if relay.URL == "" {
		if st := a.GetSessionTransport(); st != nil && st.GetPeerID() == s.sessionPid {
			return st, st.AwaitReady(ctx)
		}
	}
	if _, _, err := a.ensureSessionTransport(ctx, s.GetPrivKey(), relay.URL, relay.SigningEnvPrefix); err != nil {
		return nil, err
	}
	st := a.GetSessionTransport()
	if st == nil {
		return nil, ErrNoSessionTransport
	}
	return st, st.AwaitReady(ctx)
}
