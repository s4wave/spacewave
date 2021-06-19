package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// applyWorldOpResolver is a resolver for the ApplyWorldOp directive.
type applyWorldOpResolver struct {
	worldOpHandlers []ApplyWorldOpFunc
}

// NewApplyWorldOpResolver constructs a new resolver with a static operation handler list.
func NewApplyWorldOpResolver(worldOpHandlers []ApplyWorldOpFunc) directive.Resolver {
	return &applyWorldOpResolver{worldOpHandlers: worldOpHandlers}
}

// Resolve resolves the values, emitting them to the handler.
func (r *applyWorldOpResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	for _, cb := range r.worldOpHandlers {
		if cb != nil {
			var val ApplyWorldOpValue = cb
			_, _ = handler.AddValue(val)
		}
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*applyWorldOpResolver)(nil))
