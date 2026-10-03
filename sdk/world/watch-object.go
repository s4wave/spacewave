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
	// Acquire the watched World object and release it when the watch ends.
	objState, found, err := ws.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Track the last emitted block while observing object revisions.
	var lastSent T
	for {
		// Stop the object watch when its context is canceled.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Read the object revision that the next wait must advance past.
		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		// Read the block from the current object state.
		state, err := read(ctx, objState)
		if err != nil {
			return err
		}

		// Emit the block and retain it when its value changes.
		changed := any(lastSent) == nil || !state.EqualVT(lastSent)
		if err := emit(state, changed); err != nil {
			return err
		}
		if changed {
			lastSent = state
		}

		// Wait for the object to publish its next revision.
		if _, err := objState.WaitRev(ctx, rev+1, false); err != nil {
			return err
		}
	}
}
