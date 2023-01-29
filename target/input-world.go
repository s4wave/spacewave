package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/world"
)

// Validate validates the input world object.
func (i *InputWorld) Validate() error {
	if i.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	return nil
}

// ResolveValue resolves the InputWorld to a InputValueWorld.
//
// if lookupImmediate is set, looks up the world engine immediately
// otherwise, uses a BusEngine to look up the world engine on-demand.
func (i *InputWorld) ResolveValue(ctx context.Context, b bus.Bus) (InputValueWorld, func(), error) {
	engineID := i.GetEngineId()

	// lookup the world on the bus
	if i.GetLookupImmediate() {
		v, _, ref, err := world.ExLookupWorldEngine(ctx, b, false, engineID, nil)
		if err != nil {
			return nil, nil, err
		}
		ws := world.NewEngineWorldState(ctx, v, true)
		return NewInputValueWorld(v, ws), ref.Release, nil
	}

	// deferred lookup
	eng := world.NewBusEngine(ctx, b, engineID)
	ws := world.NewEngineWorldState(ctx, eng, true)
	return NewInputValueWorld(eng, ws), eng.Close, nil
}
