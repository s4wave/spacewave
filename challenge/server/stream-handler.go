package auth_challenge_server

import (
	"context"
	"io"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/bifrost/link"
)

// streamHandler handles HandleMountedStream directives
type streamHandler struct {
	// c is the controller
	c *Controller
	// sess is the session
	sess *auth_challenge.Session
}

// newStreamHandler builds a new stream handler
func newStreamHandler(
	c *Controller,
) *streamHandler {
	return &streamHandler{c: c}
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (s *streamHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	s.c.le.WithField("protocol-id", ms.GetProtocolID()).
		Info("auth client stream opened (by them)")

	sess := auth_challenge.NewSession(ms.GetStream())
	defer ms.GetStream().Close()
	s.sess = sess

	remotePeerID := ms.GetPeerID()
	peerRef := s.c.addPeerReference(remotePeerID, s)
	defer s.c.releasePeerReference(peerRef, s)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg := &auth_challenge.Msg{}
		if err := sess.ReadMsg(msg); err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}

		if err := peerRef.HandleMsg(msg); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*streamHandler)(nil))
