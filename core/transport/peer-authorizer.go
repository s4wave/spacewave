package transport

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
)

// ErrNoPeerAuthorizer refuses remote peers when the transport's owner supplied
// no rule for admitting them.
var ErrNoPeerAuthorizer = errors.New("session transport has no peer authorizer")

// PeerAuthorizationWatch runs until ctx ends and calls changed after each
// change that may alter which peers the authorizer admits. It returns nil
// when no change can occur.
type PeerAuthorizationWatch func(ctx context.Context, changed func()) error

// StreamRefuser reports a refusal to the remote peer in the protocol's own
// framing. The stream closes after it returns.
type StreamRefuser func(ctx context.Context, ms link.MountedStream, err error)

// WithPeerAuthorizer supplies the rule that admits remote peers to links and
// services that require authorization, such as the UDP listener.
func WithPeerAuthorizer(authorize transport_controller.PeerAuthorizer) SessionTransportOption {
	return func(t *SessionTransport) {
		t.authorizePeer = authorize
	}
}

// WithPeerAuthorizationWatch supplies the events that change the authorizer's
// answers. While the transport runs, each event re-authorizes the open gated
// links and admitted streams and closes those of refused peers.
func WithPeerAuthorizationWatch(watch PeerAuthorizationWatch) SessionTransportOption {
	return func(t *SessionTransport) {
		t.watchPeers = watch
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

// Reauthorize applies the admission rule again to every gated link and
// admitted stream, and closes those whose remote peer it now refuses.
func (t *SessionTransport) Reauthorize(ctx context.Context) {
	// Recheck the links of every gated transport.
	var controllers []*transport_controller.Controller
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		controllers = slices.Clone(t.linkControllers)
	})
	for _, ctrl := range controllers {
		ctrl.Reauthorize(ctx)
	}

	// Capture the admitted streams of each remote peer.
	streams := make(map[peer.ID][]*admittedStream)
	t.streamsMtx.Lock()
	for strm := range t.streams {
		streams[strm.remotePeer] = append(streams[strm.remotePeer], strm)
	}
	t.streamsMtx.Unlock()

	// Close the streams of each refused peer, unless ctx ended the check.
	for remotePeer, peerStreams := range streams {
		err := t.AuthorizePeer(ctx, remotePeer)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		for _, strm := range peerStreams {
			t.le.WithError(err).
				WithField("protocol-id", strm.protocolID).
				WithField("remote-peer", remotePeer.String()).
				Warn("remote peer no longer authorized, closing stream")
			_ = strm.Close()
		}
	}
}

// ServeAuthorized handles protocolID streams to this transport's peer. Each
// stream reaches handler only after AdmitStream admits its remote peer, and
// closes if a later Reauthorize refuses the peer. A refused stream goes to
// refuse, when set, and then closes. Handlers see the stream as a plain
// stream.Stream and must close it when done. The handler stays registered
// until release is called or the transport stops.
func (t *SessionTransport) ServeAuthorized(
	protocolID protocol.ID,
	handler link.MountedStreamHandler,
	refuse StreamRefuser,
) (func(), error) {
	b := t.GetChildBus()
	if b == nil {
		return nil, errors.New("session transport is not running")
	}
	authorized := &authorizedStreamHandler{t: t, next: handler, refuse: refuse}
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
	// t supplies the admission rule and tracks admitted streams.
	t *SessionTransport
	// next handles admitted streams.
	next link.MountedStreamHandler
	// refuse reports refusals to the remote peer; nil closes without a report.
	refuse StreamRefuser
}

// HandleMountedStream authorizes the remote peer without blocking the caller.
func (h *authorizedStreamHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	go func() {
		// Admit the stream, reporting a refusal to the peer before closing.
		le := h.t.le.
			WithField("protocol-id", ms.GetProtocolID()).
			WithField("remote-peer", ms.GetPeerID().String())
		strm, err := h.t.AdmitStream(ctx, ms)
		admitted := &admittedMountedStream{MountedStream: ms, strm: strm}
		if err != nil {
			le.WithError(err).Warn("refused stream")
			if h.refuse != nil {
				h.refuse(ctx, admitted, err)
			}
			_ = strm.Close()
			return
		}

		// Hand the admitted stream on, closing it if the handler fails.
		if err := h.next.HandleMountedStream(ctx, admitted); err != nil {
			le.WithError(err).Warn("handle admitted stream")
			_ = strm.Close()
		}
	}()
	return nil
}

// AdmitStream tracks the stream of ms, then authorizes its remote peer. The
// caller uses the returned stream in place of ms.GetStream(): a later
// Reauthorize that refuses the peer closes it, and closing it ends the
// tracking. A refused stream is returned too, so the caller can report the
// refusal before closing it.
func (t *SessionTransport) AdmitStream(ctx context.Context, ms link.MountedStream) (stream.Stream, error) {
	// Track the stream before authorizing, so a Reauthorize meanwhile checks it.
	strm := t.trackStream(ms)
	return strm, t.AuthorizePeer(ctx, ms.GetPeerID())
}

// trackStream records ms until its stream closes.
func (t *SessionTransport) trackStream(ms link.MountedStream) *admittedStream {
	// Wrap the stream so its Close leaves the tracked set.
	strm := &admittedStream{
		Stream:     ms.GetStream(),
		t:          t,
		remotePeer: ms.GetPeerID(),
		protocolID: ms.GetProtocolID(),
	}

	// Add it to the set Reauthorize checks.
	t.streamsMtx.Lock()
	if t.streams == nil {
		t.streams = make(map[*admittedStream]struct{})
	}
	t.streams[strm] = struct{}{}
	t.streamsMtx.Unlock()
	return strm
}

// admittedStream is a gated stream that leaves the tracked set on Close.
type admittedStream struct {
	stream.Stream
	// t tracks the stream.
	t *SessionTransport
	// remotePeer is the peer the stream was admitted for.
	remotePeer peer.ID
	// protocolID is the stream's protocol, for logs.
	protocolID protocol.ID
}

// Close forgets the stream and closes it.
func (s *admittedStream) Close() error {
	s.t.streamsMtx.Lock()
	delete(s.t.streams, s)
	s.t.streamsMtx.Unlock()
	return s.Stream.Close()
}

// admittedMountedStream presents a mounted stream through its tracked stream.
type admittedMountedStream struct {
	link.MountedStream
	// strm is the tracked stream.
	strm stream.Stream
}

// GetStream returns the tracked stream.
func (m *admittedMountedStream) GetStream() stream.Stream {
	return m.strm
}

// _ is a type assertion
var (
	_ link.MountedStreamHandler = (*authorizedStreamHandler)(nil)
	_ link.MountedStream        = (*admittedMountedStream)(nil)
	_ stream.Stream             = (*admittedStream)(nil)
)
