package world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/hash"
)

// guardCursor makes every block transaction cursor builds, in every bucket it
// reaches, write through a guardedStore that checks with check.
func guardCursor(ctx context.Context, cursor *bucket_lookup.Cursor, check func(context.Context) error) error {
	return cursor.WrapTransactionStore(ctx, func(_ context.Context, store block.StoreOps) (block.StoreOps, error) {
		return &guardedStore{store: store, check: check}, nil
	})
}

// guardedStore is the block store behind a guarded state's cursors. Putting a
// block adds content under its own hash and changes no World object: a block
// changes the World only once a guarded write, such as SetRootRef, references
// it.
// Removing a block, moving a named root and recording a completion proof can
// each change what committed World state reads, so each is checked as an
// unscoped write first.
type guardedStore struct {
	// store is the wrapped store.
	store block.StoreOps
	// check describes an unscoped write to the guard.
	check func(context.Context) error
}

// GetHashType returns the wrapped store's hash type.
func (s *guardedStore) GetHashType() hash.HashType {
	return s.store.GetHashType()
}

// GetSupportedFeatures returns the wrapped store's features.
func (s *guardedStore) GetSupportedFeatures() block.StoreFeature {
	return s.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope that is guarded too.
func (s *guardedStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := s.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &guardedStore{store: store, check: s.check}, release, nil
}

// PutBlock puts a block.
func (s *guardedStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch puts the blocks, checking a batch that removes any.
func (s *guardedStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if err := s.checkEntries(ctx, entries); err != nil {
		return nil, err
	}
	return s.store.PutBlockBatch(ctx, entries)
}

// GetBlock reads a block.
func (s *guardedStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.store.GetBlock(ctx, ref)
}

// GetStoredBlock reads a block with its stored metadata.
func (s *guardedStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return s.store.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks a block.
func (s *guardedStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks each block.
func (s *guardedStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock checks the removal as an unscoped write.
func (s *guardedStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	return s.store.RmBlock(ctx, ref)
}

// StatBlock reads a block's metadata.
func (s *guardedStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.store.StatBlock(ctx, ref)
}

// Sync fences the wrapped store.
func (s *guardedStore) Sync(ctx context.Context) (bool, error) {
	return s.store.Sync(ctx)
}

// GetRoot reads a named root.
func (s *guardedStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return s.store.GetRoot(ctx, name)
}

// ListRoots lists the named roots.
func (s *guardedStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return s.store.ListRoots(ctx, prefix, fn)
}

// SetRoots checks a change that moves a root or removes a block as an
// unscoped write.
func (s *guardedStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if len(roots) != 0 {
		if err := s.check(ctx); err != nil {
			return err
		}
	} else if err := s.checkEntries(ctx, entries); err != nil {
		return err
	}
	return s.store.SetRoots(ctx, entries, roots)
}

// checkEntries checks entries that remove a block as an unscoped write.
func (s *guardedStore) checkEntries(ctx context.Context, entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			return s.check(ctx)
		}
	}
	return nil
}

// HoldsRoots reports whether the wrapped store holds roots.
func (s *guardedStore) HoldsRoots() bool {
	return block.HoldsRoots(s.store)
}

// PinRoot pins a root in the wrapped store. A pin only keeps a block.
func (s *guardedStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.store, ref)
}

// OpenStage opens a stage on the wrapped store, guarded too.
func (s *guardedStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := block.OpenStage(ctx, s.store)
	if err != nil {
		return nil, nil, err
	}
	return &guardedStore{store: store, check: s.check}, release, nil
}

// ReleaseRoots drops this stage's own holds, which keep nothing a committed
// World object references alive.
func (s *guardedStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.store, refs)
}

// MarkRootsComplete checks a completion proof as an unscoped write: a false
// proof hides the blocks under it from every later copy.
func (s *guardedStore) MarkRootsComplete(ctx context.Context, refs []*block.BlockRef) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	return block.MarkRootsComplete(ctx, s.store, refs)
}

// RootsComplete reads the wrapped store's completion proofs.
func (s *guardedStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, s.store, refs)
}

// HoldCompleteRoots holds proved roots in the wrapped stage.
func (s *guardedStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, s.store, refs)
}

// EnsureDecodedBlockCacheFresh refreshes the wrapped store's decoded cache.
func (s *guardedStore) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	if freshener, ok := s.store.(block.DecodedBlockCacheFreshener); ok {
		return freshener.EnsureDecodedBlockCacheFresh(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.StoreOps                   = (*guardedStore)(nil)
	_ block.LeasedRoots                = (*guardedStore)(nil)
	_ block.RootProofs                 = (*guardedStore)(nil)
	_ block.DecodedBlockCacheFreshener = (*guardedStore)(nil)
)
