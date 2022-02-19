package forge_target

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
)

// LookupTarget looks up a Target in the world.
func LookupTarget(ctx context.Context, ws world.WorldState, objKey string) (*Target, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, err
	}
	var tgt *Target
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		tgt, err = UnmarshalTarget(bcs)
		return err
	})
	return tgt, err
}
