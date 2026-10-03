package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// engineStorage is the engine's writable storage. Only World stages build on
// it, so every write outside a transaction belongs to a stage.
type engineStorage struct {
	// e is the engine whose storage this is.
	e *Engine
}

// BuildStorageCursor builds a writable cursor to the world storage.
func (s engineStorage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return s.e.buildStorageCursor(ctx)
}

// AccessWorldState builds a writable cursor with an optional ref.
func (s engineStorage) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return s.e.accessStorage(ctx, ref, cb)
}

// setReadOnlyCursor makes cursor reject block writes in every bucket it
// reaches. A stage can still open on its store and write through the stage.
func setReadOnlyCursor(ctx context.Context, cursor *bucket_lookup.Cursor) {
	_ = cursor.WrapTransactionStore(ctx, wrapReadOnly)
}

// wrapReadOnly wraps store to reject block writes. It never fails.
func wrapReadOnly(_ context.Context, store block.StoreOps) (block.StoreOps, error) {
	return block.NewReadOnlyStore(store), nil
}

// _ is a type assertion
var _ world.WorldStorage = engineStorage{}
