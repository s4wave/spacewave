//go:build !js

package wasm

import (
	"slices"
	"time"

	"github.com/s4wave/spacewave/net/peer"
)

// ResourceConnectionTiming records the devtool-to-browser resource connection
// lifecycle for one TestSession.
type ResourceConnectionTiming struct {
	StartedAt   time.Time
	CompletedAt time.Time
	Err         string

	PeerWaits      []PeerWaitTiming
	Attempts       []ResourceConnectionAttemptTiming
	StartupReloads int
}

// PeerWaitTiming records one wait for a browser peer mount observation.
type PeerWaitTiming struct {
	StartedAt           time.Time
	CompletedAt         time.Time
	PeerID              peer.ID
	ObservationSequence uint64
	ObservationAge      time.Duration
	Err                 string
}

// ResourceConnectionAttemptTiming records one Resource SDK connection attempt
// against a browser peer.
type ResourceConnectionAttemptTiming struct {
	StartedAt   time.Time
	CompletedAt time.Time
	PeerID      peer.ID
	Err         string
}

// Elapsed returns the total measured resource connection duration.
func (t ResourceConnectionTiming) Elapsed() time.Duration {
	if t.StartedAt.IsZero() || t.CompletedAt.IsZero() {
		return 0
	}
	return t.CompletedAt.Sub(t.StartedAt)
}

func (s *TestSession) beginResourceConnectionTiming(start time.Time) {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	s.resourceTiming = ResourceConnectionTiming{StartedAt: start}
}

func (s *TestSession) finishResourceConnectionTiming(end time.Time, err error) {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	s.resourceTiming.CompletedAt = end
	if err != nil {
		s.resourceTiming.Err = err.Error()
	}
}

func (s *TestSession) recordPeerWaitTiming(
	start time.Time,
	end time.Time,
	obs BrowserPeerObservation,
	err error,
) {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	entry := PeerWaitTiming{
		StartedAt:           start,
		CompletedAt:         end,
		PeerID:              obs.PeerID,
		ObservationSequence: obs.Sequence,
	}
	if !obs.ObservedAt.IsZero() {
		entry.ObservationAge = end.Sub(obs.ObservedAt)
	}
	if err != nil {
		entry.Err = err.Error()
	}
	s.resourceTiming.PeerWaits = append(s.resourceTiming.PeerWaits, entry)
}

func (s *TestSession) recordResourceConnectionAttemptTiming(
	start time.Time,
	end time.Time,
	browserPeer peer.ID,
	err error,
) {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	entry := ResourceConnectionAttemptTiming{
		StartedAt:   start,
		CompletedAt: end,
		PeerID:      browserPeer,
	}
	if err != nil {
		entry.Err = err.Error()
	}
	s.resourceTiming.Attempts = append(s.resourceTiming.Attempts, entry)
}

func (s *TestSession) recordResourceStartupReload() {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	s.resourceTiming.StartupReloads++
}

// ResourceConnectionTiming returns a snapshot of the latest resource
// connection timing for the session.
func (s *TestSession) ResourceConnectionTiming() ResourceConnectionTiming {
	s.timingMu.Lock()
	defer s.timingMu.Unlock()

	out := s.resourceTiming
	out.PeerWaits = slices.Clone(s.resourceTiming.PeerWaits)
	out.Attempts = slices.Clone(s.resourceTiming.Attempts)
	return out
}
