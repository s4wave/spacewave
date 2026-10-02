package resource_client

import "github.com/aperturerobotics/starpc/srpc"

// failedResponseStream refuses response delivery while preserving the real stream's lifetime.
type failedResponseStream struct {
	// Stream carries the real request and invocation lifetime.
	srpc.Stream
	// err is the response transport failure.
	err error
}

// MsgSend refuses the response without transferring its published child IDs.
func (s *failedResponseStream) MsgSend(srpc.Message) error { return s.err }
