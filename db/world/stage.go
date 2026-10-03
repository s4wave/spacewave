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
//
// A stage that outlives many builds, such as one held by a long-lived store,
// calls ReleaseRoots after each adopting transaction returns, so it holds only
// the builds still in flight.
type WorldStage interface {
	// WorldStorage builds cursors whose writes the stage owns.
	WorldStorage

	// ReleaseRoots drops the stage's ownership of roots built through it. Call
	// it once the transaction that references them has returned. Each root
	// then survives only through a committed parent. The stage stays open.
	ReleaseRoots(ctx context.Context, roots []*block.BlockRef) error

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

// NewTransactionStage returns a stage over a write transaction's storage.
// The transaction adopts the build when it commits, after the caller would
// release a stage, so writes go through the transaction's own storage and
// Release does nothing.
func NewTransactionStage(storage WorldStorage) WorldStage {
	return passthroughStage{WorldStorage: storage}
}

// OpenWorldStorage opens storage for building through ws. A writable ws opens
// a stage that owns the build; a read-only ws returns its own storage, so
// reads work and writes fail. Release the result after the build is adopted
// or abandoned.
func OpenWorldStorage(ctx context.Context, ws WorldState) (WorldStage, error) {
	if ws.GetReadOnly() {
		return passthroughStage{WorldStorage: ws}, nil
	}
	return ws.StageWorldState(ctx)
}

// OpenStagedCursor opens storage for building through ws with
// OpenWorldStorage and returns a cursor at ref with the stage that owns its
// writes. Releasing the stage releases the cursor first; release it after the
// build is adopted or abandoned.
func OpenStagedCursor(ctx context.Context, ws WorldState, ref *bucket.ObjectRef) (*bucket_lookup.Cursor, WorldStage, error) {
	// Open the storage that owns the cursor's writes.
	stage, err := OpenWorldStorage(ctx, ws)
	if err != nil {
		return nil, nil, err
	}

	// Build the cursor and follow it to ref.
	storageRoot, err := stage.BuildStorageCursor(ctx)
	if err != nil {
		stage.Release()
		return nil, nil, err
	}
	cursor, err := storageRoot.FollowRef(ctx, ref)
	if err != nil {
		storageRoot.Release()
		stage.Release()
		return nil, nil, err
	}
	return cursor, &cursorStage{WorldStage: stage, cursors: []*bucket_lookup.Cursor{cursor, storageRoot}}, nil
}

// cursorStage releases cursors before the stage they write through.
type cursorStage struct {
	WorldStage
	cursors []*bucket_lookup.Cursor
}

// Release releases the cursors, then the stage.
func (s *cursorStage) Release() {
	for _, cursor := range s.cursors {
		cursor.Release()
	}
	s.WorldStage.Release()
}

// passthroughStage uses storage that already owns or rejects its writes.
type passthroughStage struct {
	WorldStorage
}

// ReleaseRoots does nothing: the wrapped storage owns its writes.
func (passthroughStage) ReleaseRoots(context.Context, []*block.BlockRef) error {
	return nil
}

// Release does nothing: the wrapped storage owns its writes.
func (passthroughStage) Release() {}

// storageStage implements WorldStage over World storage.
type storageStage struct {
	// storage builds the cursors this stage wraps.
	storage WorldStorage

	// mtx guards stores, releases and released.
	mtx sync.Mutex
	// stores holds the store of every volume stage this scope opened.
	stores []block.StoreOps
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

// ReleaseRoots drops the ownership of roots in every volume stage the scope
// opened. A stage that does not own a root ignores it.
func (s *storageStage) ReleaseRoots(ctx context.Context, roots []*block.BlockRef) error {
	// Take the open stages; a released scope owns nothing.
	s.mtx.Lock()
	stores := s.stores
	s.mtx.Unlock()

	// Release the roots in each stage.
	for _, store := range stores {
		if err := block.ReleaseRoots(ctx, store, roots); err != nil {
			return err
		}
	}
	return nil
}

// Release releases every volume stage the scope opened.
func (s *storageStage) Release() {
	// Mark the scope released and take its stages.
	s.mtx.Lock()
	releases := s.releases
	s.stores, s.releases, s.released = nil, nil, true
	s.mtx.Unlock()

	// Release the stages outside the lock.
	for _, release := range releases {
		release()
	}
}

// stageCursor routes the cursor's block transactions through volume stages
// the scope owns, one in each bucket the cursor reaches.
func (s *storageStage) stageCursor(ctx context.Context, cursor *bucket_lookup.Cursor) error {
	return cursor.WrapTransactionStore(ctx, s.openStage)
}

// openStage opens a stage on store and records its release with the scope.
func (s *storageStage) openStage(ctx context.Context, store block.StoreOps) (block.StoreOps, error) {
	// Open the stage on the store the cursor writes to.
	staged, release, err := block.OpenStage(ctx, store)
	if err != nil {
		return nil, err
	}

	// Record the release unless the scope already ended.
	s.mtx.Lock()
	if s.released {
		s.mtx.Unlock()
		release()
		return nil, block.ErrStageReleased
	}
	s.stores = append(s.stores, staged)
	s.releases = append(s.releases, release)
	s.mtx.Unlock()
	return staged, nil
}

// _ is a type assertion
var (
	_ WorldStage = (*storageStage)(nil)
	_ WorldStage = passthroughStage{}
)
