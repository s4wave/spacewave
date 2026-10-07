package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
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

// publishSessionTransport exposes this transport after its child bus is ready.
func (t *SessionTransport) publishSessionTransport() (func(), error) {
	// Standalone transports have no parent to publish discovery on.
	if t.parentBus == nil {
		return func() {}, nil
	}
	return t.parentBus.AddHandler(directive.NewFuncHandler(func(_ context.Context, instance directive.Instance) ([]directive.Resolver, error) {
		dir, ok := instance.GetDirective().(LookupSessionTransport)
		if !ok || dir.SessionPeerID() != t.peerID {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]*SessionTransport{t}), nil)
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
