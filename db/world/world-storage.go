package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// BuildCursorFn builds an object cursor.
type BuildCursorFn func(ctx context.Context) (*bucket_lookup.Cursor, error)

// NewCursorWorldStorage constructs a WorldStorage from an object cursor builder.
func NewCursorWorldStorage(buildCursor BuildCursorFn) WorldStorage {
	return &cursorWorldStorage{buildCursorFn: buildCursor}
}

// NewWorldStorageFromCursor builds a WorldStorage from an existing borrowed cursor.
func NewWorldStorageFromCursor(cursor *bucket_lookup.Cursor) WorldStorage {
	return NewCursorWorldStorage(func(ctx context.Context) (*bucket_lookup.Cursor, error) {
		if cursor == nil {
			return nil, ErrWorldStorageUnavailable
		}
		return cursor.Clone(), nil
	})
}

// NewAccessWorldStateFunc constructs an AccessWorldStateFunc from an existing cursor.
func NewAccessWorldStateFunc(cursor *bucket_lookup.Cursor) AccessWorldStateFunc {
	st := NewWorldStorageFromCursor(cursor)
	return st.AccessWorldState
}

// BuildStorageCursor builds a raw cursor to the world storage with an empty ref.
func (s *cursorWorldStorage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return s.buildRaw(ctx)
}

// BuildOwnedLookupCursor builds an owned lookup cursor at ref.
func (s *cursorWorldStorage) BuildOwnedLookupCursor(ctx context.Context, ref *bucket.ObjectRef) (*OwnedLookupCursor, error) {
	return s.buildOwned(ctx, ref)
}

// AccessWorldState builds a borrowed access value with an optional ref.
func (s *cursorWorldStorage) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*WorldAccess) error,
) error {
	if cb == nil {
		return nil
	}
	return s.access(ctx, ref, cb)
}

var _ WorldStorage = ((*cursorWorldStorage)(nil))
