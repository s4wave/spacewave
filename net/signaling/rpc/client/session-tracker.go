package signaling_rpc_client

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	"github.com/sirupsen/logrus"
)

// sessionTracker wraps an ongoing session with a peer.
type sessionTracker struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// key is the string encoding of the peer id.
	key string
	// peerID is the parsed version of the peer id
	peerID peer.ID
}

// newSessionTracker constructs a new sessionTracker.
func (c *Controller) newSessionTracker(peerIDStr string) (keyed.Routine, *sessionTracker) {
	// note: we confirmed that parsePeerID is valid before adding the key
	peerID, _ := peer.IDB58Decode(peerIDStr)
	le := c.le.WithField("remote-peer-id", peerIDStr)

	sess := &sessionTracker{
		c:      c,
		le:     le,
		key:    peerIDStr,
		peerID: peerID,
	}
	return sess.execute, sess
}

// execute executes the sessionTracker.
func (s *sessionTracker) execute(ctx context.Context) error {
	// Wait for the client to be ready.
	client, err := s.c.client.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Open the signaling session with the remote peer.
	signalRef := client.AddPeerRef(s.peerID.String())
	defer signalRef.Release()

	// Add the handler directive
	if !s.c.conf.GetDisableListen() {
		sess := NewSessionWithRef(signalRef)
		di, dirRef, err := s.c.b.AddDirective(
			signaling.NewHandleSignalPeer(
				s.c.conf.GetSignalingId(),
				sess,
			),
			nil,
		)
		if err != nil {
			return err
		}
		defer di.Close()
		defer dirRef.Release()
	}

	// Wait for context to be canceled before releasing
	<-ctx.Done()
	return context.Canceled
}
