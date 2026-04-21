package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// cursorWorldStorage implements WorldStorage with a object cursor.
type cursorWorldStorage struct {
	// buildCursorFn builds a new root cursor.
	buildCursorFn BuildCursorFn
}

// BuildCursorFn builds a object cursor.
type BuildCursorFn func(ctx context.Context) (*bucket_lookup.Cursor, error)

// NewCursorWorldStorage constructs a WorldStorage from a object cursor.
func NewCursorWorldStorage(buildCursor BuildCursorFn) WorldStorage {
	return &cursorWorldStorage{buildCursorFn: buildCursor}
}

// NewWorldStorageFromCursor builds a WorldStorage from an existing cursor.
func NewWorldStorageFromCursor(cursor *bucket_lookup.Cursor) WorldStorage {
	return NewCursorWorldStorage(func(ctx context.Context) (*bucket_lookup.Cursor, error) {
		return cursor.Clone(), nil
	})
}

// NewAccessWorldStateFunc constructs an AccessWorldStateFunc from a existing cursor
func NewAccessWorldStateFunc(cursor *bucket_lookup.Cursor) AccessWorldStateFunc {
	st := NewWorldStorageFromCursor(cursor)
	return st.AccessWorldState
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (s *cursorWorldStorage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	if s.buildCursorFn == nil {
		return nil, ErrWorldStorageUnavailable
	}
	return s.buildCursorFn(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns a cursor pointing to the root world state.
// The lookup cursor will be released after cb returns.
func (s *cursorWorldStorage) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	cursor, err := s.buildCursorFn(ctx)
	if err != nil {
		return err
	}
	defer cursor.Release()

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
var _ WorldStorage = ((*cursorWorldStorage)(nil))
