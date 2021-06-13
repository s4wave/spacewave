package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/world"
)

// WorldEngineResolver resolves LookupWorldEngine with the controller engine.
type WorldEngineResolver struct {
	// c is the controller
	c *Controller
}

// NewWorldEngineResolver constructs a new dial resolver.
func NewWorldEngineResolver(c *Controller) (*WorldEngineResolver, error) {
	return &WorldEngineResolver{c: c}, nil
}

// Resolve resolves the values, emitting them to the handler.
func (r *WorldEngineResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	eng, err := r.c.GetWorldEngine(ctx)
	if err != nil {
		return err
	}

	var v world.LookupWorldEngineValue = eng
	_, _ = handler.AddValue(v)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*WorldEngineResolver)(nil))
