package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// LookupSessionTransport selects the running transport for one authenticated
// Session. The value is its *SessionTransport and is withdrawn when that
// transport stops.
type LookupSessionTransport interface {
	directive.Directive

	// SessionPeerID returns the Session whose transport is requested.
	SessionPeerID() peer.ID
}

// lookupSessionTransport implements LookupSessionTransport.
type lookupSessionTransport struct {
	// peerID is the Session whose transport is requested.
	peerID peer.ID
}

// NewLookupSessionTransport constructs a LookupSessionTransport for the
// Session with peer ID id.
func NewLookupSessionTransport(id peer.ID) LookupSessionTransport {
	return &lookupSessionTransport{peerID: id}
}

// ResolveSessionTransport borrows the Session's running transport while
// release is retained. It returns nil when no transport runs for the Session.
// Removing a resolved transport calls invalidated; callers must end the work
// that depends on it.
func ResolveSessionTransport(ctx context.Context, parent bus.Bus, id peer.ID, invalidated func()) (*SessionTransport, func(), error) {
	// Wait for the transport, or report none once the lookup goes idle.
	resolved, _, ref, err := bus.ExecWaitValue[*SessionTransport](ctx, parent,
		NewLookupSessionTransport(id), bus.ReturnIfIdle(true), invalidated, nil)
	if err != nil {
		return nil, nil, err
	}
	if resolved == nil {
		return nil, func() {}, nil
	}
	return resolved, ref.Release, nil
}

// AuthorizeAccountSessionPeer admits a Device stream through the Session
// transport for the link's local peer. The transport's account authorizer
// admits only active Sessions of that account. A missing transport refuses
// the stream.
//
// Use and close the returned stream. The transport closes an admitted stream
// if it later refuses the peer. On error the returned stream can carry the
// protocol's refusal before the caller closes it.
func AuthorizeAccountSessionPeer(ctx context.Context, b bus.Bus, ms link.MountedStream) (stream.Stream, error) {
	// Find the transport owning the local end of the link.
	st, release, err := ResolveSessionTransport(ctx, b, ms.GetLink().GetLocalPeer(), nil)
	if err != nil {
		return ms.GetStream(), err
	}
	defer release()
	if st == nil {
		return ms.GetStream(), errors.New("no session transport for the local peer")
	}

	// Track the stream for revocation and apply the account's admission rule.
	return st.AdmitStream(ctx, ms)
}

// ResolveSessionBus borrows the Session's transport bus while release is
// retained. A host without Session transports uses its supplied bus directly.
// Removing a resolved transport calls invalidated; callers must end their
// pending streams.
func ResolveSessionBus(ctx context.Context, parent bus.Bus, id peer.ID, invalidated func()) (bus.Bus, func(), error) {
	// Fall back to the supplied bus when no Session transport runs.
	resolved, release, err := ResolveSessionTransport(ctx, parent, id, invalidated)
	if err != nil {
		return nil, nil, err
	}
	if resolved == nil {
		return parent, release, nil
	}

	// Use the running transport's child bus.
	child := resolved.GetChildBus()
	if child == nil {
		release()
		return nil, nil, errors.New("session transport stopped")
	}
	return child, release, nil
}

// publishSessionTransport exposes this transport, and signs for its Session
// peer, on the parent bus after its child bus is ready.
func (t *SessionTransport) publishSessionTransport() (func(), error) {
	// Standalone transports have no parent to publish discovery on.
	if t.parentBus == nil {
		return func() {}, nil
	}
	return t.parentBus.AddHandler(directive.NewFuncHandler(func(_ context.Context, instance directive.Instance) ([]directive.Resolver, error) {
		switch dir := instance.GetDirective().(type) {
		case LookupSessionTransport:
			if dir.SessionPeerID() == t.peerID {
				return directive.R(directive.NewValueResolver([]*SessionTransport{t}), nil)
			}
		case peer.SignWithPeer:
			// The running Session signs for its peer; the key stays here.
			if res := peer.NewSignWithPeerResolver(dir, t.sessionKey); res != nil {
				return directive.R(res, nil)
			}
		}
		return nil, nil
	}))
}

// SessionPeerID is the exact Session whose transport is requested.
func (d *lookupSessionTransport) SessionPeerID() peer.ID { return d.peerID }

// Validate rejects a lookup without an authenticated identity.
func (d *lookupSessionTransport) Validate() error { return d.peerID.Validate() }

// GetValueOptions releases unused lookups immediately.
func (d *lookupSessionTransport) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{UnrefDisposeEmptyImmediate: true}
}

// IsEquivalent shares lookups for the same Session.
func (d *lookupSessionTransport) IsEquivalent(other directive.Directive) bool {
	o, ok := other.(LookupSessionTransport)
	return ok && o.SessionPeerID() == d.peerID
}

// Superceeds leaves other Sessions' transports independent.
func (d *lookupSessionTransport) Superceeds(directive.Directive) bool { return false }

// GetName identifies the transport lookup in diagnostics.
func (d *lookupSessionTransport) GetName() string { return "LookupSessionTransport" }

// GetDebugVals identifies the requested Session.
func (d *lookupSessionTransport) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"peer-id": []string{d.peerID.String()}}
}

// _ is a type assertion
var _ LookupSessionTransport = (*lookupSessionTransport)(nil)
