package forge_target

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// LookupTarget looks up a Target in the world.
func LookupTarget(ctx context.Context, ws world.WorldState, objKey string) (*Target, error) {
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return nil, err
	}
	var tgt *Target
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		tgt, err = UnmarshalTarget(ctx, bcs)
		return err
	})
	return tgt, err
}
