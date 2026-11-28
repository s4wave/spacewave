package space_world

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// ReplayCursorOperations is how many positions the saved replay may fall
// behind the last position before the Adapter saves it again.
const ReplayCursorOperations = replayCursorOperations

// ReadSavedCursor returns the replay cursor the Adapter of so saved, or nil.
func ReadSavedCursor(ctx context.Context, so sobject.SharedObject) (*ReplayCursor, error) {
	return readSavedCursor(ctx, so)
}

// DeleteSavedCursor deletes the replay cursor the Adapter of so saved.
func DeleteSavedCursor(ctx context.Context, so sobject.SharedObject) error {
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, nil)
}

// WriteSavedCursor saves cursor as the replay cursor of the Adapter of so.
func WriteSavedCursor(ctx context.Context, so sobject.SharedObject, cursor *ReplayCursor) error {
	data, err := cursor.MarshalVT()
	if err != nil {
		return err
	}
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, data)
}

// SetAdmittedHook makes fn see each candidate an Adapter keeps until the
// returned func clears it.
func SetAdmittedHook(fn func(world.Candidate)) func() {
	admittedHook.Store(&fn)
	return func() { admittedHook.Store(nil) }
}

// SetHeldHook makes fn run after each hold an Adapter takes for an admitted
// write, with whether the hold was ordered, failing the admission with its
// error, until the returned func clears it.
func SetHeldHook(fn func(ordered bool) error) func() {
	heldHook.Store(&fn)
	return func() { heldHook.Store(nil) }
}

// ForceOrderedHolds orders the holds of each Adapter opened until the returned
// func ends it, even on a SharedObject that does not report ordering its block
// store's writes.
func ForceOrderedHolds() func() {
	forceOrderedHolds.Store(true)
	return func() { forceOrderedHolds.Store(false) }
}

// ForceDeferredMoves queues the admission root moves of each Adapter opened
// until the returned func ends it, as SPACEWAVE_WORLD_DEFERRED_ROOT_MOVE=on
// does.
func ForceDeferredMoves() func() {
	forceDeferredMoves.Store(true)
	return func() { forceDeferredMoves.Store(false) }
}

// SetLandHook makes fn run before each admission's queued root moves land,
// with whether it queued one, until the returned func clears it. An error
// from fn drops every queued move unstored, as a crash before the durable
// step loses them, and fails the admission with it.
func SetLandHook(fn func(queued bool) error) func() {
	landHook.Store(&fn)
	return func() { landHook.Store(nil) }
}

// SetCheckpointStoreHook sets fn until the returned function clears it.
func SetCheckpointStoreHook(fn func(context.Context)) func() {
	checkpointStoreHook.Store(&fn)
	return func() { checkpointStoreHook.Store(nil) }
}

// WaitSigned waits until a write of eng's Space World is inside its
// QueueOperation with its operation signed, and returns the operation's hash.
// The signer publishes no event, so it checks the settle set each millisecond.
func WaitSigned(ctx context.Context, eng *Engine) ([]byte, error) {
	set := eng.a.settles
	for {
		// Find an undecided sent write whose receipt names its operation.
		var hash []byte
		set.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, ent := range set.entries {
				if !ent.told && !ent.resolved && len(ent.receipt.OpHash()) != 0 {
					hash = ent.receipt.OpHash()
					return
				}
			}
		})
		if hash != nil {
			return hash, nil
		}

		// Look again shortly.
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-time.After(time.Millisecond):
		}
	}
}
