package signaling_echo

import (
	"context"
	"unicode/utf8"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/signaling"
	"github.com/sirupsen/logrus"
)

// EchoResolver resolves HandleSignalPeer by echoing data.
type EchoResolver struct {
	// le is the logger
	le *logrus.Entry
	// dir is the directive
	dir signaling.HandleSignalPeer
}

// NewEchoResolver constructs a new dial resolver.
func NewEchoResolver(le *logrus.Entry, dir signaling.HandleSignalPeer) (*EchoResolver, error) {
	le = le.WithField("signaling-id", dir.HandleSignalingID()).
		WithField("local-peer", dir.HandleSignalPeerSession().GetLocalPeerID().String()).
		WithField("remote-peer", dir.HandleSignalPeerSession().GetRemotePeerID().String())
	return &EchoResolver{le: le, dir: dir}, nil
}

func (r *EchoResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	r.le.Debug("starting echo signaling handler")

	sess := r.dir.HandleSignalPeerSession()
	for {
		msg, err := sess.Recv(ctx)
		if err != nil {
			return err
		}

		if utf8.Valid(msg) {
			r.le.Debugf("echoing incoming utf8 message len(%v): %v", len(msg), string(msg))
		} else {
			r.le.Debugf("echoing incoming binary message len(%v)", len(msg))
		}

		if err := sess.Send(ctx, msg); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*EchoResolver)(nil))
