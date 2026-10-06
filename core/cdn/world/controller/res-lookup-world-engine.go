package cdn_world_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// worldEngineResolver publishes the mounted engine. A failed mount marks the
// lookup idle without a value, so callers that wait while idle learn that the
// Space is unreachable instead of blocking until it returns.
type worldEngineResolver struct {
	// ctr publishes the outcome of the latest mount attempt.
	ctr *ccontainer.CContainer[*mountState]
}

// Resolve follows mount outcomes until ctx is canceled.
func (r *worldEngineResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var state *mountState
	for {
		var err error
		state, err = r.ctr.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		_ = handler.ClearValues()
		if state == nil {
			// The mount was withdrawn; the next attempt publishes again.
			handler.MarkIdle(false)
			continue
		}
		if state.engine != nil {
			_, _ = handler.AddValue(state.engine)
		}
		handler.MarkIdle(true)
	}
}

// _ is a type assertion
var _ directive.Resolver = (*worldEngineResolver)(nil)
