package store

import (
	"context"
	"sort"

	"github.com/s4wave/spacewave/db/block"
)

// writebackBlock is one resident block handed to writeback.
type writebackBlock struct {
	// key is the kvfile index key.
	key string
	// ref is the block ref parsed from key.
	ref *block.BlockRef
	// off is the packfile offset of the encoded block value.
	off int64
	// size is the encoded block value size in bytes.
	size int64
	// spans cover [off, off+size). Span bytes are immutable, so they stay
	// readable after eviction removes the spans from the store.
	spans []*span
}

// prepareWritebackLocked collects the unpublished blocks overlapping
// [start, end) whose bytes are fully resident, marks them published, and
// returns the job that writes them back. Returns nil when there is nothing
// to write. The caller runs the job with startOwnerWork after releasing bcast.
func (e *PackReader) prepareWritebackLocked(start, end int64) func() {
	// Require an open pack reader with a configured writeback target.
	if e.closed || e.writebackTarget == nil || e.writebackCtx == nil {
		return nil
	}

	// Select unpublished blocks whose complete bytes are resident.
	pos := sort.Search(len(e.entriesByOff), func(i int) bool {
		_, entryEnd := entryExtent(e.entriesByOff[i])
		return entryEnd > start
	})
	var blocks []writebackBlock
	for _, entry := range e.entriesByOff[pos:] {
		// Restrict writeback candidates to the fetched byte interval.
		eOff, eEnd := entryExtent(entry)
		if eOff >= end {
			break
		}

		// Skip blocks already handed to the writeback target.
		key := string(entry.GetKey())
		if _, ok := e.published[key]; ok {
			continue
		}

		// Require complete resident bytes for the candidate block.
		spans, covered := e.collectSpansLocked(eOff, eEnd)
		if !covered {
			continue
		}

		// Validate the block key and reserve its writeback publication.
		ref, err := parseBlockRef(entry)
		if err != nil {
			continue
		}
		e.published[key] = struct{}{}
		blocks = append(blocks, writebackBlock{
			key:   key,
			ref:   ref,
			off:   eOff,
			size:  eEnd - eOff,
			spans: spans,
		})
	}
	if len(blocks) == 0 {
		return nil
	}

	// Retain the reader lifetime for the selected writeback batch.
	ctx, target := e.writebackCtx, e.writebackTarget
	e.workCount++
	e.writebackRunning++
	return func() { e.writeback(ctx, target, blocks) }
}

// writeback verifies blocks and writes them to target in one batch.
//
// A block that fails verification is skipped; a later read of it rejects the
// same bytes and drops them. A failed batch unmarks its blocks so a later
// fetch covering them publishes them again.
func (e *PackReader) writeback(ctx context.Context, target block.StoreOps, blocks []writebackBlock) {
	// Release the reader lifetime after the writeback batch finishes.
	defer e.finishOwnerWork()

	// Decode and verify resident blocks before persisting the writeback batch.
	entries := make([]*block.PutBatchEntry, 0, len(blocks))
	var failed []string
	for _, b := range blocks {
		value := make([]byte, b.size)
		copySpans(value, b.spans, b.off)
		stored, err := decodeVerifiedBlock(b.ref, value)
		if err != nil {
			failed = append(failed, b.key)
			continue
		}
		entries = append(entries, &block.PutBatchEntry{Ref: b.ref, Data: stored.Data, Refs: stored.Refs})
	}

	// Persist the verified blocks in one target-store batch.
	var err error
	if len(entries) != 0 {
		err = target.PutBlockBatch(ctx, entries)
	}

	// Publish writeback counters and release failed block reservations.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Account for completed writeback and release failed publication reservations.
		e.writebackRunning--
		e.verifyFailures += uint64(len(failed))
		for _, key := range failed {
			delete(e.published, key)
		}
		if err != nil {
			e.writebackErrors++
			for _, b := range blocks {
				delete(e.published, b.key)
			}
		} else {
			e.writebackCount += uint64(len(entries))
		}
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after publishing the writeback result.
	if notify != nil {
		notify()
	}
}
