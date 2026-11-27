package space_world

import (
	"context"

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

// SetCheckpointStoreHook sets fn until the returned function clears it.
func SetCheckpointStoreHook(fn func(context.Context)) func() {
	checkpointStoreHook.Store(&fn)
	return func() { checkpointStoreHook.Store(nil) }
}
