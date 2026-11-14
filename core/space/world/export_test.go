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

// WriteSavedCursor saves cursor as the replay cursor of the Adapter of so.
func WriteSavedCursor(ctx context.Context, so sobject.SharedObject, cursor *ReplayCursor) error {
	data, err := cursor.MarshalVT()
	if err != nil {
		return err
	}
	return writeLocalState(ctx, so, replayCursorStoreID, replayCursorKey, data)
}
