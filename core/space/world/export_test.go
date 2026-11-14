package space_world

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
)

// ReadSavedCursor returns the replay cursor the Adapter of so saved, or nil.
func ReadSavedCursor(ctx context.Context, so sobject.SharedObject) (*ReplayCursor, error) {
	return readSavedCursor(ctx, so)
}

// DeleteSavedCursor deletes the replay cursor the Adapter of so saved.
func DeleteSavedCursor(ctx context.Context, so sobject.SharedObject) error {
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, nil)
}
