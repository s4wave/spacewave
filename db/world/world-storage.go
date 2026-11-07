package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// cursorWorldStorage implements WorldStorage with a object cursor.
type cursorWorldStorage struct {
	// cursor is the root cursor, cloned for each access.
	cursor *bucket_lookup.Cursor
}

// NewWorldStorageFromCursor builds a WorldStorage from an existing cursor.
func NewWorldStorageFromCursor(cursor *bucket_lookup.Cursor) WorldStorage {
	return &cursorWorldStorage{cursor: cursor}
}

// NewAccessWorldStateFunc constructs an AccessWorldStateFunc from an existing cursor.
func NewAccessWorldStateFunc(cursor *bucket_lookup.Cursor) AccessWorldStateFunc {
	return NewWorldStorageFromCursor(cursor).AccessWorldState
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (s *cursorWorldStorage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return s.cursor.Clone(), nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns a cursor pointing to the root world state.
// The lookup cursor will be released after cb returns.
func (s *cursorWorldStorage) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	// Acquire the World storage cursor for this access.
	cursor := s.cursor.Clone()
	defer cursor.Release()

	// Follow the requested reference when it differs from the World root.
	ncs := cursor
	if !cursor.GetRef().EqualsRef(ref) {
		var err error
		ncs, err = cursor.FollowRef(ctx, ref)
		if err != nil {
			return err
		}
		defer ncs.Release()
	}

	return cb(ncs)
}

// _ is a type assertion
var _ WorldStorage = (*cursorWorldStorage)(nil)
