package world_control

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// NewWaitForStateHandler constructs a WatchLoopHandler to wait for a state.
func NewWaitForStateHandler(
	cb func(
		ctx context.Context,
		ws world.WorldState,
		// may be nil if not found
		obj world.ObjectState,
		rootCs *block.Cursor,
		rev uint64,
	) (bool, error),
) WatchLoopHandler {
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
		berr = ws.AccessWorldState(ctx, rootRef, func(bls *world.WorldAccess) error {
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
//
// The returned object state is the caller's to own: release it with
// world.ReleaseObjectState. The handle the watch loop hands its handler lives
// for one iteration only, so this acquires a fresh one after the loop ends
// rather than keeping the handler's.
func WaitForObjectRev(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objKey string,
	rev uint64,
) (world.ObjectState, error) {
	var reached bool
	lp := NewWatchLoop(
		le,
		objKey,
		NewWaitForStateHandler(func(_ context.Context, _ world.WorldState, obj world.ObjectState, rootCs *block.Cursor, crev uint64) (bool, error) {
			if obj == nil || crev < rev {
				return true, nil
			}
			reached = true
			return false, nil
		}),
	)
	if err := lp.Execute(ctx, ws); err != nil {
		return nil, err
	}
	if !reached {
		return nil, nil
	}
	out, found, err := ws.GetObject(ctx, objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return out, nil
}
