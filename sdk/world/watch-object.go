package s4wave_world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// watchBlock is the value contract WatchWorldObject needs from a block:
// wire encoding, change detection, and clone-on-send.
type watchBlock[T any] interface {
	block.Block
	EqualVT(other T) bool
	CloneVT() T
}

// ReadWorldBlock reads one block of type T from an object state. It returns
// a nil T without error when the object carries no block yet.
func ReadWorldBlock[T block.Block](
	ctx context.Context,
	objState world.ObjectState,
	ctor func() block.Block,
) (T, error) {
	var out T
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		out, uerr = block.UnmarshalBlock[T](ctx, bcs, ctor)
		return uerr
	})
	return out, err
}

// WatchWorldObject streams re-reads of one world object after each revision.
// read runs once per revision against the object handle; emit runs after each
// read with changed reporting whether the value differs (EqualVT) from the
// previous revision. The watch ends when ctx is canceled.
func WatchWorldObject[T watchBlock[T]](
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	read func(ctx context.Context, objState world.ObjectState) (T, error),
	emit func(state T, changed bool) error,
) error {
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent T
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		state, err := read(ctx, objState)
		if err != nil {
			return err
		}

		changed := any(lastSent) == nil || !state.EqualVT(lastSent)
		if err := emit(state, changed); err != nil {
			return err
		}
		if changed {
			lastSent = state
		}

		if _, err := objState.WaitRev(ctx, rev+1, false); err != nil {
			return err
		}
	}
}
