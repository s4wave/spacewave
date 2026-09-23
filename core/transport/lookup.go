package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
)

// LookupSessionBus selects the running transport for one authenticated Session.
// The value is its bus.Bus and is withdrawn when that transport stops.
type LookupSessionBus interface {
	directive.Directive
	SessionPeerID() peer.ID
}

type lookupSessionBus struct {
	peerID peer.ID
}

// ResolveSessionBus borrows the Session's transport while release is retained.
// A host without Session transports uses its supplied bus directly. Removing a
// resolved transport calls invalidated; callers must end their pending streams.
func ResolveSessionBus(ctx context.Context, parent bus.Bus, id peer.ID, invalidated func()) (bus.Bus, func(), error) {
	resolved, _, ref, err := bus.ExecWaitValue[bus.Bus](ctx, parent,
		&lookupSessionBus{peerID: id}, bus.ReturnIfIdle(true), invalidated, nil)
	if err != nil {
		return nil, nil, err
	}
	if resolved == nil {
		return parent, func() {}, nil
	}
	return resolved, ref.Release, nil
}

// publishSessionBus exposes this transport after its child bus is ready.
func (t *SessionTransport) publishSessionBus(child bus.Bus) (func(), error) {
	// Standalone transports have no parent to publish discovery on.
	if t.parentBus == nil {
		return func() {}, nil
	}
	return t.parentBus.AddHandler(directive.NewFuncHandler(func(_ context.Context, instance directive.Instance) ([]directive.Resolver, error) {
		dir, ok := instance.GetDirective().(LookupSessionBus)
		if !ok || dir.SessionPeerID() != t.peerID {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]bus.Bus{child}), nil)
	}))
}

// SessionPeerID is the exact Session whose transport is requested.
func (d *lookupSessionBus) SessionPeerID() peer.ID { return d.peerID }

// Validate rejects a lookup without an authenticated identity.
func (d *lookupSessionBus) Validate() error { return d.peerID.Validate() }

// GetValueOptions releases unused lookups immediately.
func (d *lookupSessionBus) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{UnrefDisposeEmptyImmediate: true}
}

// IsEquivalent shares lookups for the same Session.
func (d *lookupSessionBus) IsEquivalent(other directive.Directive) bool {
	o, ok := other.(LookupSessionBus)
	return ok && o.SessionPeerID() == d.peerID
}

// Superceeds leaves other Sessions' transports independent.
func (d *lookupSessionBus) Superceeds(directive.Directive) bool { return false }

// GetName identifies the transport lookup in diagnostics.
func (d *lookupSessionBus) GetName() string { return "LookupSessionBus" }

// GetDebugVals identifies the requested Session.
func (d *lookupSessionBus) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"peer-id": []string{d.peerID.String()}}
}
