package store

import (
	"context"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// getBlock is the top-level engine read path.
//
// It reads the target bytes from resident spans, fetching the semantic window
// around the target on a miss, and checks them against the block ref. Fetched
// bytes can be evicted before the read copies them, so the read fetches at
// most twice. A hash mismatch drops the spans holding the target so a later
// read fetches it again.
//
// key is the index key of ref's hash. Returns nil when the pack index does not
// hold the block.
func (e *PackReader) getBlock(ctx context.Context, key []byte, ref *block.BlockRef) (*block.StoredBlock, error) {
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return nil, err
	}

	var (
		off, end    int64
		windowStart int64
		windowEnd   int64
		found       bool
	)
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		var entry *kvfile.IndexEntry
		entry, found = e.findEntryByKeyLocked(key)
		if !found {
			return
		}
		off, end = entryExtent(entry)
		windowStart, windowEnd = e.semanticWindowLocked(entry)
	})
	if !found {
		return nil, nil
	}

	for range 3 {
		data, ok := e.readResidentRange(off, end)
		if !ok {
			if err := e.ensureResident(ctx, windowStart, windowEnd, false); err != nil {
				return nil, err
			}
			continue
		}
		stored, err := decodeVerifiedBlock(ref, data)
		if err != nil {
			e.dropRange(off, end)
		}
		return stored, err
	}
	return nil, errors.Errorf("packfile block %x not resident after fetch", key)
}

// dropRange removes the resident spans overlapping [start, end) and counts a
// verification failure.
func (e *PackReader) dropRange(start, end int64) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.verifyFailures++
		i := e.spanIndexLocked(start)
		for i < len(e.spans) && e.spans[i].off < end {
			e.removeSpanLocked(e.spans[i])
		}
		broadcast()
	})
}

// decodeVerifiedBlock decodes a pack value and checks its data against ref.
func decodeVerifiedBlock(ref *block.BlockRef, value []byte) (*block.StoredBlock, error) {
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, err
	}
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, errors.Wrapf(block.ErrBlockRefMismatch, "packfile block %s", ref.MarshalString())
	}
	return stored, nil
}

func (e *PackReader) getBlockExists(ctx context.Context, key []byte) (bool, error) {
	if err := e.ensureIndexLoaded(ctx); err != nil {
		return false, err
	}

	var found bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, found = e.findEntryByKeyLocked(key)
	})
	return found, nil
}

// statBlock reports whether the pack holds the block. The data size is
// unknown without decoding the value, so it is -1.
func (e *PackReader) statBlock(ctx context.Context, key []byte, ref *block.BlockRef) (*block.BlockStat, error) {
	found, err := e.getBlockExists(ctx, key)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
}
