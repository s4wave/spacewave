package block_store_filesegment

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// Behind returns primary with the linked files behind it: writes and roots go
// to primary, and a read that primary misses, or fails, reads the files. A
// file is its block's copy, so a file hit fills nothing into primary.
func (s *Store) Behind(primary block.StoreOps) block.StoreOps {
	return &behind{StoreOps: primary, files: s}
}

// behind reads a store, then the linked files.
type behind struct {
	// StoreOps is the primary store, which takes every write.
	block.StoreOps
	// files serves the linked files.
	files *Store
}

// BeginReadOperation scopes the primary store. The files hold no scope.
func (b *behind) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	primary, release, err := b.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &behind{StoreOps: primary, files: b.files}, release, nil
}

// GetBlock reads primary, then the files.
func (b *behind) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := b.StoreOps.GetBlock(ctx, ref)
	if err == nil && found {
		return data, true, nil
	}
	stored, err := b.fallback(ctx, ref, err)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock reads primary, then the files.
func (b *behind) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := b.StoreOps.GetStoredBlock(ctx, ref)
	if err == nil && stored != nil {
		return stored, nil
	}
	return b.fallback(ctx, ref, err)
}

// fallback reads the files after primary missed ref or failed with
// primaryErr, which it returns when the files miss too.
func (b *behind) fallback(ctx context.Context, ref *block.BlockRef, primaryErr error) (*block.StoredBlock, error) {
	stored, err := b.files.GetStoredBlock(ctx, ref)
	if err == nil && stored != nil {
		return stored, nil
	}
	if primaryErr != nil {
		return nil, primaryErr
	}
	return nil, err
}

// GetBlockExists checks primary, then the files.
func (b *behind) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Check primary.
	found, err := b.StoreOps.GetBlockExists(ctx, ref)
	if err == nil && found {
		return true, nil
	}

	// Check the files, reporting primary's failure when they miss.
	inFiles, filesErr := b.files.GetBlockExists(ctx, ref)
	if filesErr == nil && inFiles {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, filesErr
}

// GetBlockExistsBatch checks primary for every ref, then the files for the
// refs primary did not confirm.
func (b *behind) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check primary.
	found, err := b.StoreOps.GetBlockExistsBatch(ctx, refs)
	out := make([]bool, len(refs))
	if err == nil {
		copy(out, found)
	}

	// Check the files for the rest, reporting primary's failure after.
	for i, ok := range out {
		if ok {
			continue
		}
		inFiles, filesErr := b.files.GetBlockExists(ctx, refs[i])
		if filesErr != nil {
			return nil, filesErr
		}
		out[i] = inFiles
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StatBlock stats primary, then the files.
func (b *behind) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Stat primary.
	stat, err := b.StoreOps.StatBlock(ctx, ref)
	if err == nil && stat != nil {
		return stat, nil
	}

	// Stat the files, reporting primary's failure when they miss.
	fileStat, filesErr := b.files.StatBlock(ctx, ref)
	if filesErr == nil && fileStat != nil {
		return fileStat, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, filesErr
}

// _ is a type assertion
var _ block.StoreOps = (*behind)(nil)
