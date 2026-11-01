package store

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"slices"
	"sort"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// ensureIndexLoaded loads the kvfile index for this pack if not already loaded.
//
// The raw index-tail cache is consulted first. On a miss, the reader reads the
// tail straight from the transport, outside the span store, so tail bytes
// never count as resident payload. Parsed entries are runtime-only views
// rebuilt from the raw tail. After a successful load any block already fully
// covered by resident spans is handed to writeback.
func (e *PackReader) ensureIndexLoaded(ctx context.Context) error {
	// Reject canceled demand before touching the reader.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Join the loaded index or an in-flight load, or start one.
	var loaded, closed, started bool
	var waitCh chan struct{}
	var cache IndexCache
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Reject index loading after the pack reader closes.
		if e.closed {
			closed = true
			return
		}

		// Reuse the validated index already held by the reader.
		if e.indexLoaded {
			loaded = true
			return
		}

		// Join the current index load instead of starting another.
		if e.indexLoadCh != nil {
			waitCh = e.indexLoadCh
			return
		}

		// Register an index load under the reader lifetime.
		e.indexLoadCh = make(chan struct{})
		waitCh = e.indexLoadCh
		cache = e.indexCache
		e.workCount++
		started = true
	})
	if closed {
		return ErrPackReaderClosed
	}
	if loaded {
		return nil
	}
	if started {
		e.startIndexLoad(cache)
	}

	// Wait for the load to finish, the caller to cancel, or the reader to close.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.ctx.Done():
		return ErrPackReaderClosed
	case <-waitCh:
	}

	// Read the completed index load result under the reader state lock.
	var err error
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = e.indexLoadErr
	})
	return err
}

// startIndexLoad loads and publishes the pack index under the PackReader lifetime.
func (e *PackReader) startIndexLoad(cache IndexCache) {
	startOwnerWork(func() {
		// Hold the reader open until this job finishes.
		defer e.finishOwnerWork()

		// Read the index from the cache, or from the pack tail on a miss.
		var tail []byte
		var entries []*kvfile.IndexEntry
		var err error
		if cache != nil {
			cached, ok, cacheErr := cache.Get(e.ctx, e.packID)
			if cacheErr != nil {
				e.recordIndexCacheReadError()
			} else if ok {
				entries, cacheErr = e.parseIndexTail(cached)
				if cacheErr != nil {
					e.recordIndexCacheReadError()
				}
				if cacheErr == nil {
					e.recordIndexCacheHit()
				}
			} else {
				e.recordIndexCacheMiss()
			}
		}

		// Load and cache the remote index tail when cached entries are unavailable.
		if entries == nil {
			before := e.snapshotFetchedBytes()
			tail, entries, err = e.readIndexTailEntries(e.ctx)
			e.recordRemoteIndexLoad(e.snapshotFetchedBytes() - before)
			if err == nil && cache != nil {
				if cacheErr := cache.Set(e.ctx, e.packID, tail); cacheErr != nil {
					e.recordIndexCacheWriteError()
				}
			}
		}

		// Publish the loaded index unless the reader closed during the load.
		var writeback func()
		var budget *residentBudget
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// A closed reader drops the index; otherwise publish it.
			loadCh := e.indexLoadCh
			if e.closed {
				err = ErrPackReaderClosed
			}
			if err == nil {
				e.setIndexEntriesLocked(entries)
				e.indexUseSeq = e.nextUseSeqLocked()
				writeback = e.prepareWritebackLocked(0, e.size)
				e.indexLoaded = true
				budget = e.budget
			} else {
				e.indexLoaded = false
			}
			e.indexLoadErr = err
			e.indexLoadCh = nil
			if loadCh != nil {
				close(loadCh)
			}
			broadcast()
		})
		if writeback != nil {
			startOwnerWork(writeback)
		}

		// Drop cold indexes above the shared entry limit.
		if budget != nil {
			budget.reclaimIndexes()
		}
	})
}

// withIndex loads the pack index and calls fn under the reader state lock
// with the index loaded, so fn reads entries that a drop cannot clear under
// it. A drop between the load and the lock loads the index again.
func (e *PackReader) withIndex(ctx context.Context, fn func()) error {
	for {
		if err := e.ensureIndexLoaded(ctx); err != nil {
			return err
		}
		var ran bool
		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if !e.indexLoaded {
				return
			}
			e.indexUseSeq = e.nextUseSeqLocked()
			fn()
			ran = true
		})
		if ran {
			return nil
		}
	}
}

// lastIndexUse returns the budget clock at the last use of the loaded index,
// and false when no index is loaded.
func (e *PackReader) lastIndexUse() (uint64, bool) {
	var seq uint64
	var ok bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed || !e.indexLoaded {
			return
		}
		seq, ok = e.indexUseSeq, true
	})
	return seq, ok
}

// dropIndex drops the parsed index when it is unused since seq. The next use
// loads it again, from the index cache when one is set.
func (e *PackReader) dropIndex(seq uint64) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed || !e.indexLoaded || e.indexUseSeq != seq {
			return
		}
		e.setIndexEntriesLocked(nil)
		e.indexLoaded = false
		e.indexDrops++
	})
}

// snapshotFetchedBytes returns the bytes requested by completed fetches.
func (e *PackReader) snapshotFetchedBytes() int64 {
	var bytes int64
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		bytes = e.fetchBytes
	})
	return bytes
}

// recordIndexCacheHit counts a valid cached tail and notifies stats consumers.
func (e *PackReader) recordIndexCacheHit() {
	// Publish the index cache counter change under the reader state lock.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheHits++
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after releasing the reader state lock.
	if notify != nil {
		notify()
	}
}

// recordIndexCacheMiss counts an absent cached tail and notifies stats consumers.
func (e *PackReader) recordIndexCacheMiss() {
	// Publish the index cache counter change under the reader state lock.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheMisses++
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after releasing the reader state lock.
	if notify != nil {
		notify()
	}
}

// recordIndexCacheReadError counts an unreadable or invalid cached tail.
func (e *PackReader) recordIndexCacheReadError() {
	// Publish the index cache counter change under the reader state lock.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheReadErrors++
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after releasing the reader state lock.
	if notify != nil {
		notify()
	}
}

// recordIndexCacheWriteError counts a failed attempt to cache a validated tail.
func (e *PackReader) recordIndexCacheWriteError() {
	// Publish the index cache counter change under the reader state lock.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheWriteErrors++
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after releasing the reader state lock.
	if notify != nil {
		notify()
	}
}

// recordRemoteIndexLoad counts one remote index load and its transport bytes.
func (e *PackReader) recordRemoteIndexLoad(bytes int64) {
	// Keep remote index byte accounting nonnegative.
	if bytes < 0 {
		bytes = 0
	}

	// Publish the remote index load totals under the reader state lock.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Account for the completed remote index load and wake stats watchers.
		e.remoteIndexLoads++
		e.remoteIndexBytes += bytes
		e.lastRemoteIndexBytes = bytes
		notify = e.statsChanged
		broadcast()
	})

	// Notify stats observers after releasing the reader state lock.
	if notify != nil {
		notify()
	}
}

// readIndexTailEntries reads the raw kvfile index tail from the transport and
// returns it with its parsed entries.
func (e *PackReader) readIndexTailEntries(ctx context.Context) ([]byte, []*kvfile.IndexEntry, error) {
	// Reject a negative pack size before reading its index tail.
	if e.size < 0 {
		return nil, nil, errors.Errorf("negative pack size %d", e.size)
	}
	tail, err := packfile.ReadIndexTail(ctx, uint64(e.size), e.fetchIndexTail)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read kvfile index tail")
	}
	entries, err := e.parseIndexTail(tail)
	return tail, entries, err
}

// fetchIndexTail reads length bytes at off for the index tail as foreground
// demand. The bytes go to the caller alone; they never enter the span store.
func (e *PackReader) fetchIndexTail(ctx context.Context, off int64, length int) ([]byte, error) {
	// Fetch through the demand gate so background fetches yield to it.
	var gate *demandGate
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		gate = e.gate
	})
	leave, err := gate.enter(ctx, false, nil)
	if err != nil {
		return nil, err
	}
	data, err := e.transport.Fetch(ctx, off, length)
	leave()
	if err != nil {
		return nil, err
	}

	// Count the read as index I/O.
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		notify = e.recordIndexTailFetchLocked(fetchKey{off: off, size: length}, len(data))
		broadcast()
	})
	if notify != nil {
		notify()
	}
	return data, nil
}

// parseIndexTail validates the tail against the manifest size and block count.
func (e *PackReader) parseIndexTail(tail []byte) ([]*kvfile.IndexEntry, error) {
	// Reject a negative pack size before parsing cached index bytes.
	if e.size < 0 {
		return nil, errors.Errorf("negative pack size %d", e.size)
	}

	// Build the index reader and require the manifest entry count.
	reader, err := kvfile.BuildReaderWithIndexTail(tail, uint64(e.size))
	if err != nil {
		return nil, errors.Wrap(err, "build kvfile reader from index tail")
	}
	if reader.Size() != e.blockCount {
		return nil, errors.Errorf("index entry count %d != manifest block count %d", reader.Size(), e.blockCount)
	}

	// Ensure the index entry count fits the in-memory slice length.
	count := reader.Size()
	if count > uint64(math.MaxInt) {
		return nil, errors.Errorf("index entry count %d overflows int", count)
	}

	// Copy index entries out of the validated tail reader.
	entries := make([]*kvfile.IndexEntry, 0, count)
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		entries = append(entries, ie.CloneVT())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan index entries")
	}

	// Validate block keys and payload extents against the index-tail boundary.
	tailStart := uint64(e.size) - uint64(len(tail))
	if err := validateIndexEntries(entries, tailStart, e.blockCount); err != nil {
		return nil, err
	}
	return entries, nil
}

// validateIndexEntries checks key order, block references, and payload extents.
func validateIndexEntries(entries []*kvfile.IndexEntry, tailStart uint64, blockCount uint64) error {
	// Require one index entry for each block declared in the manifest.
	if uint64(len(entries)) != blockCount {
		return errors.Errorf("index entry count %d != manifest block count %d", len(entries), blockCount)
	}

	// Validate every index key and block payload extent.
	var prev []byte
	for i, entry := range entries {
		// Require an index record with a nonempty key.
		if entry == nil {
			return errors.Errorf("nil index entry at %d", i)
		}
		key := entry.GetKey()
		if len(key) == 0 {
			return errors.Errorf("empty index key at %d", i)
		}

		// Require strictly increasing index keys that encode block references.
		if prev != nil && bytes.Compare(prev, key) >= 0 {
			return errors.Errorf("duplicate or unsorted index key at %d", i)
		}
		prev = key
		if _, err := parseBlockRef(entry); err != nil {
			return errors.Wrapf(err, "parse index key at %d", i)
		}

		// Require each block payload to end before the index tail without overflow.
		off := entry.GetOffset()
		size := entry.GetSize()
		if size > math.MaxUint64-off {
			return errors.Errorf("index entry %d offset %d size %d overflows", i, off, size)
		}
		if off+size > tailStart {
			return errors.Errorf("index entry %d end %d exceeds tail start %d", i, off+size, tailStart)
		}
	}
	return nil
}

// setIndexEntriesLocked stores the index entries sorted by offset and by key,
// or clears them when entries is nil.
func (e *PackReader) setIndexEntriesLocked(entries []*kvfile.IndexEntry) {
	// Order index entries by payload offset for resident window planning.
	byOff := slices.Clone(entries)
	slices.SortFunc(byOff, func(a, b *kvfile.IndexEntry) int {
		return cmp.Compare(a.GetOffset(), b.GetOffset())
	})

	// Order index entries by block key for existence and metadata probes.
	byKey := slices.Clone(entries)
	slices.SortFunc(byKey, func(a, b *kvfile.IndexEntry) int {
		return bytes.Compare(a.GetKey(), b.GetKey())
	})

	// Publish both index orders into the reader state and charge the budget.
	e.budget.indexUsed.Add(int64(len(byKey) - len(e.entriesByKey)))
	e.entriesByOff = byOff
	e.entriesByKey = byKey
}

// findEntryByKeyLocked binary-searches the key-sorted index.
func (e *PackReader) findEntryByKeyLocked(key []byte) (*kvfile.IndexEntry, bool) {
	return binarySearchEntriesByKey(e.entriesByKey, key)
}

// semanticWindowLocked returns the byte span covering the target block plus
// every neighbor block fully contained within the configured writeback
// window on each side.
//
// When no writeback target is configured the window shrinks to just the
// target bytes.
func (e *PackReader) semanticWindowLocked(target *kvfile.IndexEntry) (int64, int64) {
	// Use the target block extent when neighbor writeback is disabled.
	targetOff, targetEnd := entryExtent(target)
	start := targetOff
	end := targetEnd
	if e.writebackTarget == nil || e.writebackWindow <= 0 {
		return start, end
	}

	// Compute the desired neighbor window around the target block.
	half := e.writebackWindow / 2
	intendedStart := int64(0)
	if targetOff > half {
		intendedStart = targetOff - half
	}
	intendedEnd := targetEnd + half

	// Expand the payload extent to include complete blocks within the neighbor window.
	pos := sort.Search(len(e.entriesByOff), func(i int) bool {
		return int64(e.entriesByOff[i].GetOffset()) >= intendedStart //nolint:gosec // validateIndexEntries bounds offsets by the int64 pack size.
	})
	for ; pos < len(e.entriesByOff); pos++ {
		// Skip neighbor blocks that extend beyond the desired window.
		eOff, eEnd := entryExtent(e.entriesByOff[pos])
		if eOff >= intendedEnd {
			break
		}
		if eEnd > intendedEnd {
			continue
		}

		// Include the complete neighbor block in the payload fetch extent.
		start = min(start, eOff)
		end = max(end, eEnd)
	}
	return start, end
}

// entryExtent returns the packfile byte interval of a validated index entry.
func entryExtent(entry *kvfile.IndexEntry) (int64, int64) {
	off := int64(entry.GetOffset())          //nolint:gosec // validateIndexEntries bounds offsets by the int64 pack size.
	return off, off + int64(entry.GetSize()) //nolint:gosec // validated entries have non-overflowing extents within the int64 pack size.
}

// parseBlockRef builds a block ref from a kvfile index entry key.
func parseBlockRef(entry *kvfile.IndexEntry) (*block.BlockRef, error) {
	h, err := packfile.ParseBlockKey(entry.GetKey())
	if err != nil {
		return nil, err
	}
	return block.NewBlockRef(h), nil
}
