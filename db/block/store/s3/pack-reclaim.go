//go:build !tinygo

package block_store_s3

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// Reclaim drops the blocks live reports dead from the packfiles listed before
// fence runs.
//
// The pass lists the entries, calls fence, and then judges only the listed
// packfiles. A writer that may still reference a dead block must upload it
// again after fence returns, so that upload lands in a packfile the pass does
// not judge. live reports for each ref whether its block is still reachable.
//
// A packfile whose dead blocks hold at least half its value bytes is rewritten
// with only its live blocks, or deleted when none is live. Compaction merges
// the rewritten packfiles later. After a pass, no listed packfile is half
// dead. A packfile another writer merged first is skipped; its blocks wait for
// the next pass.
func (s *PackStore) Reclaim(
	ctx context.Context,
	fence func(context.Context) error,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
) error {
	// Exclude compaction for the pass.
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Snapshot the listed packfiles before the fence.
	if err := s.listEntries(ctx, s.listings.Load()); err != nil {
		return err
	}
	var listed []*packfile.PackfileEntry
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		listed = slices.Collect(maps.Values(s.entries))
	})
	if err := fence(ctx); err != nil {
		return errors.Wrap(err, "reclaim fence")
	}

	// Judge and rewrite each listed packfile.
	for _, entry := range listed {
		err := s.reclaimPack(ctx, entry, live)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// reclaimPack rewrites the packfile of entry without its dead blocks when they
// hold at least half its value bytes. Returns ErrNotFound when the packfile is
// gone.
func (s *PackStore) reclaimPack(
	ctx context.Context,
	entry *packfile.PackfileEntry,
	live func(context.Context, []*block.BlockRef) ([]bool, error),
) error {
	// Ask live about each block the packfile index names.
	index, err := s.readPackIndex(ctx, entry)
	if err != nil {
		return err
	}
	refs := make([]*block.BlockRef, len(index))
	for i, ie := range index {
		h, err := packfile.ParseBlockKey(ie.GetKey())
		if err != nil {
			return errors.Wrap(err, "packfile "+entry.GetId())
		}
		refs[i] = block.NewBlockRef(h)
	}
	alive, err := live(ctx, refs)
	if err != nil {
		return errors.Wrap(err, "check live blocks")
	}
	if len(alive) != len(refs) {
		return errors.Errorf("live returned %d results for %d blocks", len(alive), len(refs))
	}

	// Keep the packfile unless its dead blocks hold half its value bytes.
	var total, dead uint64
	var keys []string
	for i, ie := range index {
		total += ie.GetSize()
		if alive[i] {
			keys = append(keys, string(ie.GetKey()))
		} else {
			dead += ie.GetSize()
		}
	}
	if dead == 0 || dead*2 < total {
		return nil
	}

	// Rewrite the live blocks, in key order, and drop the packfile.
	var output *packfile.PackfileEntry
	if len(keys) != 0 {
		values, err := s.readPack(ctx, entry.GetId())
		if err != nil {
			return err
		}
		output, err = s.writeValues(ctx, keys, values)
		if err != nil {
			return errors.Wrap(err, "write reclaimed packfile")
		}
	}
	return s.replace(ctx, output, []string{entry.GetId()})
}

// readPackIndex reads the key index of the packfile of entry through ranged
// reads, without its block values.
func (s *PackStore) readPackIndex(ctx context.Context, entry *packfile.PackfileEntry) ([]*kvfile.IndexEntry, error) {
	// Open the packfile for ranged reads.
	size := entry.GetSizeBytes()
	pack, err := s.openPack(entry.GetId(), int64(size)) //nolint:gosec // a packfile is at most writer.DefaultMaxPackBytes.
	if err != nil {
		return nil, err
	}
	defer pack.Close()

	// Read the index at its tail.
	_, tail, err := kvfile.ReadIndexTail(pack.ReaderAt(ctx), size)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index "+entry.GetId())
	}
	rd, err := kvfile.BuildReaderWithIndexTail(tail, size)
	if err != nil {
		return nil, errors.Wrap(err, "open packfile index "+entry.GetId())
	}

	// Collect its entries.
	var index []*kvfile.IndexEntry
	err = rd.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		index = append(index, ie.CloneVT())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan packfile index "+entry.GetId())
	}
	return index, nil
}
