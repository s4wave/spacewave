package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// applyObjectOpResolver is a resolver for the ApplyObjectOp directive.
type applyObjectOpResolver struct {
	objectOpHandlers []ApplyObjectOpFunc
}

// NewApplyObjectOpResolver constructs a new resolver with a static operation handler list.
func NewApplyObjectOpResolver(objectOpHandlers []ApplyObjectOpFunc) directive.Resolver {
	return &applyObjectOpResolver{objectOpHandlers: objectOpHandlers}
}

// Resolve resolves the values, emitting them to the handler.
func (r *applyObjectOpResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	for _, cb := range r.objectOpHandlers {
		if cb != nil {
			var val ApplyObjectOpValue = cb
			_, _ = handler.AddValue(val)
		}
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*applyObjectOpResolver)(nil))
