package world

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// WorldStage is World storage whose writes a staging scope owns.
//
// Blocks written through the stage stay alive until Release, so a caller can
// build object data outside a World transaction and adopt it in a later one.
// Hold the stage until the transaction that references the built roots has
// returned, then call Release. Blocks no committed parent references are
// reclaimed after Release, including when the adopting transaction failed.
type WorldStage interface {
	// WorldStorage builds cursors whose writes the stage owns.
	WorldStorage

	// Release ends the stage. Writes through its cursors fail afterward.
	Release()
}

// NewStorageStage stages the writes made through cursors from storage.
//
// Each cursor's block store opens its own volume stage, so a cursor following
// a reference into another bucket stages there too. A store without root
// retention writes directly, as it does outside a stage.
func NewStorageStage(storage WorldStorage) WorldStage {
	return &storageStage{storage: storage}
}

// storageStage implements WorldStage over World storage.
type storageStage struct {
	// storage builds the cursors this stage wraps.
	storage WorldStorage

	// mtx guards releases and released.
	mtx sync.Mutex
	// releases holds the release of every volume stage this scope opened.
	releases []func()
	// released is set once Release has run.
	released bool
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// Release the cursor independently of the stage.
func (s *storageStage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	// Build the cursor from the wrapped storage.
	cursor, err := s.storage.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}

	// Route its writes through a volume stage.
	if err := s.stageCursor(ctx, cursor); err != nil {
		cursor.Release()
		return nil, err
	}
	return cursor, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// The cursor is released after cb returns; the stage keeps its writes.
func (s *storageStage) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return s.storage.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		if err := s.stageCursor(ctx, cursor); err != nil {
			return err
		}
		return cb(cursor)
	})
}

// Release releases every volume stage the scope opened.
func (s *storageStage) Release() {
	// Mark the scope released and take its stages.
	s.mtx.Lock()
	releases := s.releases
	s.releases, s.released = nil, true
	s.mtx.Unlock()

	// Release the stages outside the lock.
	for _, release := range releases {
		release()
	}
}

// stageCursor opens a volume stage on the cursor's store and routes the
// cursor's block transactions through it.
func (s *storageStage) stageCursor(ctx context.Context, cursor *bucket_lookup.Cursor) error {
	// Open the stage on the store the cursor writes to.
	staged, release, err := block.OpenStage(ctx, cursor.GetBlockStore())
	if err != nil {
		return err
	}

	// Record the release unless the scope already ended.
	s.mtx.Lock()
	if s.released {
		s.mtx.Unlock()
		release()
		return block.ErrStageReleased
	}
	s.releases = append(s.releases, release)
	s.mtx.Unlock()

	// Route the cursor's writes through the stage.
	cursor.SetTransactionStore(staged)
	return nil
}

// _ is a type assertion
var _ WorldStage = (*storageStage)(nil)
