//go:build !tinygo

package block_store_s3

import (
	"context"
	"maps"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// ScanBlocks lists the bucket's packfiles and calls fn with every block they
// hold, reading each packfile whole. A block packed twice is visited twice.
// Holds off this store's compaction and reclaim during the scan; another
// writer merging the same prefix may make it fail with ErrNotFound.
func (s *PackStore) ScanBlocks(ctx context.Context, fn func(ref *block.BlockRef, stored *block.StoredBlock) error) error {
	// Keep this store's merges from deleting a packfile mid-scan.
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// List the packfiles in the bucket.
	if err := s.listEntries(ctx, s.listings.Load()); err != nil {
		return err
	}
	var ids []string
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		ids = slices.Sorted(maps.Keys(s.entries))
	})

	// Read each packfile and visit its blocks in key order.
	for _, id := range ids {
		values, err := s.readPack(ctx, id)
		if err != nil {
			return err
		}
		for _, key := range slices.Sorted(maps.Keys(values)) {
			ref, stored, err := packfile.DecodeBlockValue([]byte(key), values[key])
			if err != nil {
				return err
			}
			if err := fn(ref, stored); err != nil {
				return err
			}
		}
	}
	return nil
}
