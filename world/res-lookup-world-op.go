package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// lookupWorldOpResolver is a resolver for the LookupWorldOp directive.
type lookupWorldOpResolver struct {
	lookupOp LookupOp
}

// NewLookupWorldOpResolver constructs a new resolver with a static lookup func list.
func NewLookupWorldOpResolver(lookupOp LookupOp) directive.Resolver {
	return &lookupWorldOpResolver{lookupOp: lookupOp}
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupWorldOpResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	var val LookupWorldOpValue = r.lookupOp
	if val != nil {
		_, _ = handler.AddValue(val)
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupWorldOpResolver)(nil))
