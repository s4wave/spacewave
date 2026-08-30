package s4wave_canvas

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// LookupCanvasState loads one logical Canvas state from a World object.
func LookupCanvasState(ctx context.Context, ws world.WorldState, objKey string) (*CanvasState, error) {
	obj, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	defer world.ReleaseObjectState(obj)

	var state *CanvasState
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		state, err = UnmarshalCanvasState(ctx, bcs)
		return err
	})
	if err == nil && state == nil {
		state = &CanvasState{}
	}
	return state, err
}
