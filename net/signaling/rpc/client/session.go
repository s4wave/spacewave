package signaling_rpc_client

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
)

// Session implements signaling.Session.
type Session struct {
	ref *ClientPeerRef
}

// NewSessionWithRef wraps a ClientPeerRef into a signaling.Session.
func NewSessionWithRef(ref *ClientPeerRef) *Session {
	return &Session{ref: ref}
}

// GetLocalPeerID returns the local peer ID.
func (s *Session) GetLocalPeerID() peer.ID {
	return s.ref.GetLocalPeerID()
}

// GetRemotePeerID returns the remote peer ID.
func (s *Session) GetRemotePeerID() peer.ID {
	return s.ref.GetRemotePeerID()
}

// Send transmits a message to the remote peer.
// Blocks until the context is canceled OR the message is acked.
func (s *Session) Send(ctx context.Context, msg []byte) error {
	_, err := s.ref.Send(ctx, msg)
	return err
}

// Recv waits for and acknowledges an incoming message from the remote peer.
func (s *Session) Recv(ctx context.Context) ([]byte, error) {
	msg, err := s.ref.Recv(ctx)
	if err != nil {
		return nil, err
	}
	return msg.GetSignedMsg().GetData(), nil
}

// _ is a type assertion
var _ signaling.SignalPeerSession = ((*Session)(nil))
