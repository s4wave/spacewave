package link_solicit

import (
	"errors"
	"sync"

	"github.com/s4wave/spacewave/net/link"
)

// SolicitMountedStream is the value type for SolicitProtocol.
// It wraps a solicited stream that was matched via hash exchange.
type SolicitMountedStream interface {
	// AcceptMountedStream claims ownership of the stream.
	// Returns (stream, false, nil) on success.
	// Returns (nil, true, nil) if already accepted by another caller.
	// Returns (nil, false, err) if the stream is closed or errored.
	AcceptMountedStream() (link.MountedStream, bool, error)
}

// solicitMountedStream implements SolicitMountedStream.
type solicitMountedStream struct {
	ms link.MountedStream

	// mu guards err and accepted.
	mu       sync.Mutex
	err      error
	accepted bool
}

// NewSolicitMountedStream constructs a new SolicitMountedStream value.
func NewSolicitMountedStream(ms link.MountedStream) SolicitMountedStream {
	return &solicitMountedStream{ms: ms}
}

// NewSolicitMountedStreamWithErr constructs an errored SolicitMountedStream.
func NewSolicitMountedStreamWithErr(err error) SolicitMountedStream {
	return &solicitMountedStream{err: err}
}

// AcceptMountedStream claims ownership of the stream.
func (s *solicitMountedStream) AcceptMountedStream() (link.MountedStream, bool, error) {
	// Serialize callers claiming the solicited stream with Close.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return the solicitation failure before claiming its stream.
	if s.err != nil {
		return nil, false, s.err
	}

	// Claim the stream only for the first accepting caller.
	if s.accepted {
		return nil, true, nil
	}
	s.accepted = true
	return s.ms, false, nil
}

// IsAccepted returns whether the stream has been accepted.
func (s *solicitMountedStream) IsAccepted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

// Close closes the stream if it has not been accepted.
// Returns true if the stream was closed (not accepted).
func (s *solicitMountedStream) Close() bool {
	// Serialize stream closure with callers claiming the stream.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve streams already claimed or absent from the solicitation.
	if s.accepted || s.ms == nil {
		return false
	}

	// Close the unclaimed stream and retain the solicitation failure.
	s.ms.GetStream().Close()
	s.err = errSolicitationClosed
	return true
}

// errSolicitationClosed is returned when the solicitation was closed.
var errSolicitationClosed = errors.New("solicitation closed")

// _ is a type assertion
var _ SolicitMountedStream = (*solicitMountedStream)(nil)
