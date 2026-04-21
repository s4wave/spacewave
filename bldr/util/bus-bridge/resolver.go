package bus_bridge

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// BusBridgeResolver resolves by forwarding the directive to a target bus.
type BusBridgeResolver struct {
	// target is the target bus
	target bus.Bus
	// dir is the directive
	dir directive.Directive
}

// NewBusBridgeResolver constructs a new BusBridgeResolver.
func NewBusBridgeResolver(target bus.Bus, dir directive.Directive) *BusBridgeResolver {
	return &BusBridgeResolver{
		target: target,
		dir:    dir,
	}
}

// Resolve resolves the values, emitting them to the handler.
func (r *BusBridgeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	if r.target == nil || r.dir == nil {
		return nil
	}

	disposedCb := func() {
		_ = handler.ClearValues()
	}
	_, diRef, err := r.target.AddDirective(r.dir, bus.NewPassThruHandler(handler, disposedCb))
	if err != nil {
		return err
	}
	handler.AddResolverRemovedCallback(diRef.Release)

	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*BusBridgeResolver)(nil))
