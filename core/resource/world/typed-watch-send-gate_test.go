//go:build !js

package resource_world_test

import "github.com/aperturerobotics/starpc/srpc"

// typedWatchSendGate blocks one actual transport send after initial availability.
type typedWatchSendGate struct {
	// Stream delegates the real Resource RPC transport.
	srpc.Stream
	// armed enables backpressure after the test adopts initial availability.
	armed <-chan struct{}
	// gated records the single sender's completed gate transition.
	gated bool
	// blocked notifies the test that backpressure reached the transport.
	blocked chan<- struct{}
	// release removes backpressure without changing registry or World state.
	release <-chan struct{}
}

// MsgSend pauses the second response while lifecycle ownership continues independently.
func (s *typedWatchSendGate) MsgSend(message srpc.Message) error {
	// Gate only the first response after the initial snapshot.
	if !s.gated {
		select {
		case <-s.armed:
			s.gated = true
			s.blocked <- struct{}{}
			select {
			case <-s.release:
			case <-s.Context().Done():
				return s.Context().Err()
			}
		default:
		}
	}

	// Forward through the original real Resource transport.
	return s.Stream.MsgSend(message)
}

// _ is a type assertion.
var _ srpc.Stream = (*typedWatchSendGate)(nil)
