package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/world"
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
	// Resolve the selected engine eagerly when the input requests immediate access.
	engineID := i.GetEngineId()
	if i.GetLookupImmediate() {
		v, _, ref, err := world.ExLookupWorldEngine(ctx, b, false, engineID, nil)
		if err != nil {
			return nil, nil, err
		}
		ws := world.NewEngineWorldState(v, true)
		return NewInputValueWorld(engineID, v, ws), ref.Release, nil
	}

	// Retain the selected ID with the deferred engine and release its demand with the input.
	eng := world.NewBusEngine(ctx, b, engineID)
	ws := world.NewEngineWorldState(eng, true)
	return NewInputValueWorld(engineID, eng, ws), eng.ClearContext, nil
}
