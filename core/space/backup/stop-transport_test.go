package space_backup_test

import (
	"context"
	"net/http"
)

// stopTransport models process loss just after one server call completed.
type stopTransport struct {
	// base sends each call to the fake server.
	base http.RoundTripper
	// after is the one-based response after which the process loses its context.
	after int
	// calls counts completed HTTP round trips.
	calls int
	// cancel ends the simulated process, leaving its local KV and Drive intact.
	cancel context.CancelFunc
}

// RoundTrip commits the call, then cancels at the chosen interruption boundary.
func (s *stopTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := s.base.RoundTrip(request)
	s.calls++
	if s.calls == s.after {
		s.cancel()
	}
	return response, err
}

// _ is a type assertion.
var _ http.RoundTripper = (*stopTransport)(nil)
