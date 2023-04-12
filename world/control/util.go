package world_control

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// NewWaitForStateHandler constructs an ObjectLoopHandler to wait for a state.
func NewWaitForStateHandler(
	cb func(
		ctx context.Context,
		ws world.WorldState,
		// may be nil if not found
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error),
) ObjectLoopHandler {
	return func(
		ctx context.Context,
		le *logrus.Entry,
		ws world.WorldState,
		obj world.ObjectState, // may be nil if not found
		rootRef *bucket.ObjectRef, rev uint64,
	) (waitForChanges bool, berr error) {
		if obj == nil {
			return cb(ctx, ws, nil, nil, rev)
		}
		berr = ws.AccessWorldState(ctx, rootRef, func(bls *bucket_lookup.Cursor) error {
			_, bcs := bls.BuildTransaction(nil)
			var err error
			waitForChanges, err = cb(ctx, ws, obj, bcs, rev)
			return err
		})
		return
	}
}

// WaitForObjectRev waits for the object to exist equal at or greater than the given rev.
// If rev=0, waits for the object to exist at any rev.
func WaitForObjectRev(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	rev uint64,
) (world.ObjectState, error) {
	var out world.ObjectState
	lp := NewObjectLoop(
		le,
		objKey,
		NewWaitForStateHandler(func(_ context.Context, _ world.WorldState, obj world.ObjectState, rootCs *block.Cursor, crev uint64) (bool, error) {
			if obj == nil || crev < rev {
				return true, nil
			}
			out = obj
			return false, nil
		}),
	)
	err := lp.Execute(ctx, ws)
	if err != nil {
		return nil, err
	}
	return out, nil
}
