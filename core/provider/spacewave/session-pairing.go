package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/transport"
)

// GetPairingEngine returns this unlocked Session's account enrollment owner.
func (s *Session) GetPairingEngine() (*pairing.Engine, error) {
	// Serialize engine access and reject locked or closed Sessions.
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	if s.GetPrivKey() == nil || s.lifecycleCtx.Err() != nil {
		return nil, errors.New("Session is locked or closed")
	}

	// Reuse the enrollment engine for this unlocked Session.
	if s.pairingEngine != nil {
		return s.pairingEngine, nil
	}

	// Bind a new enrollment engine to the Session lifetime.
	ctx, cancel := context.WithCancel(s.lifecycleCtx)
	a := s.tkr.a
	engine, err := pairing.NewEngine(ctx, a.le, a.p.b, s, a, s.GetPairingTransport)
	if err != nil {
		cancel()
		return nil, err
	}

	// Publish the engine and its cancellation together under the pairing lock.
	s.pairingEngine, s.pairingCancel = engine, cancel
	return engine, nil
}

// clearPairingEngine stops the pairing engine before discarding its reference.
func (s *Session) clearPairingEngine() {
	// Stop and discard the engine while excluding concurrent engine creation.
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	if s.pairingCancel != nil {
		s.pairingCancel()
	}
	s.pairingEngine, s.pairingCancel = nil, nil
}

// GetPairingTransport uses this Session's ordinary transport composition.
// Starting pairing explicitly enables the connection policy pairing requires.
func (s *Session) GetPairingTransport(ctx context.Context, relay pairing.Relay) (*transport.SessionTransport, error) {
	// Enable the Session transport before waiting for its pairing connection.
	if !s.GetDirectP2PEnabled() {
		if err := s.SetDirectP2PEnabled(ctx, true); err != nil {
			return nil, err
		}
	}

	// Wait for the Session transport while retaining the account change notification.
	a := s.tkr.a
	for {
		// Snapshot this Session's transport and the next account transport change.
		var st *transport.SessionTransport
		var state *sessionTransportState
		var wait <-chan struct{}
		a.transportBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			wait = getWait()
			state = a.sessionTransports[s.tkr.id]
			if state != nil {
				st = state.transport
			}
		})

		// Wait on the selected transport after releasing the account lock.
		if st != nil {
			return st, state.awaitReady(ctx)
		}

		// Resume when a transport is published or the pairing request ends.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}
