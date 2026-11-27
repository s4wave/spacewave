package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// WorldEngineResolver resolves LookupWorldEngine with the controller engine.
type WorldEngineResolver struct {
	// c is the controller.
	c Controller
}

// NewWorldEngineResolver constructs a new dial resolver.
func NewWorldEngineResolver(c Controller) (*WorldEngineResolver, error) {
	return &WorldEngineResolver{c: c}, nil
}

// Resolve publishes the engine the controller serves. When the controller
// replaces its engine within a run, Resolve removes the retired engine before
// the controller closes it and publishes its successor, so a lookup never
// holds a closed engine.
func (r *WorldEngineResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	replacing, ok := r.c.(ReplacingController)
	if !ok {
		eng, err := r.c.GetWorldEngine(ctx)
		if err != nil {
			return err
		}
		_, _ = handler.AddValue(eng)
		return nil
	}
	for {
		eng, release, err := replacing.AcquireWorldEngine(ctx)
		if err != nil {
			return err
		}
		valID, _ := handler.AddValue(eng)
		err = replacing.WaitWorldEngineReplaced(ctx, eng)
		_, _ = handler.RemoveValue(valID)
		release()
		if err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = (*WorldEngineResolver)(nil)
