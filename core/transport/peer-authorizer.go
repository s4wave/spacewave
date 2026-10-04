package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
)

// ErrNoPeerAuthorizer refuses remote peers when the transport's owner supplied
// no rule for admitting them.
var ErrNoPeerAuthorizer = errors.New("session transport has no peer authorizer")

// WithPeerAuthorizer supplies the rule that admits remote peers to links and
// services that require authorization, such as the UDP listener.
func WithPeerAuthorizer(authorize transport_controller.PeerAuthorizer) SessionTransportOption {
	return func(t *SessionTransport) {
		t.authorizePeer = authorize
	}
}

// AuthorizePeer applies the owner's admission rule to a remote peer. Without
// a rule every peer is refused.
func (t *SessionTransport) AuthorizePeer(ctx context.Context, remotePeer peer.ID) error {
	if t.authorizePeer == nil {
		return ErrNoPeerAuthorizer
	}
	return t.authorizePeer(ctx, remotePeer)
}

// ServeAuthorized handles protocolID streams to this transport's peer. Each
// stream reaches handler only after AuthorizePeer admits its remote peer;
// refused streams are closed. The handler stays registered until release is
// called or the transport stops.
func (t *SessionTransport) ServeAuthorized(protocolID protocol.ID, handler link.MountedStreamHandler) (func(), error) {
	b := t.GetChildBus()
	if b == nil {
		return nil, errors.New("session transport is not running")
	}
	authorized := &authorizedStreamHandler{t: t, next: handler}
	return b.AddHandler(directive.NewFuncHandler(func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
		dir, ok := di.GetDirective().(link.HandleMountedStream)
		if !ok ||
			dir.HandleMountedStreamProtocolID() != protocolID ||
			dir.HandleMountedStreamLocalPeerID() != t.peerID {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]link.MountedStreamHandler{authorized}), nil)
	}))
}

// authorizedStreamHandler admits a stream's remote peer before handing it on.
type authorizedStreamHandler struct {
	// t supplies the admission rule.
	t *SessionTransport
	// next handles admitted streams.
	next link.MountedStreamHandler
}

// HandleMountedStream authorizes the remote peer without blocking the caller.
func (h *authorizedStreamHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	go func() {
		err := h.t.AuthorizePeer(ctx, ms.GetPeerID())
		if err == nil {
			err = h.next.HandleMountedStream(ctx, ms)
		}
		if err != nil {
			h.t.le.WithError(err).
				WithField("protocol-id", ms.GetProtocolID()).
				WithField("remote-peer", ms.GetPeerID().String()).
				Warn("refused stream")
			_ = ms.GetStream().Close()
		}
	}()
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = (*authorizedStreamHandler)(nil)
