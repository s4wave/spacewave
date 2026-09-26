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

// defaultIndexTailInitialWindow is the first index-tail read size.
const defaultIndexTailInitialWindow = 256 * 1024

// ensureIndexLoaded loads the kvfile index for this pack if not already loaded.
//
// The raw index-tail cache is consulted first. On a miss, the engine slices
// the kvfile tail through its own ReaderAt so tail bytes land in the shared
// span store. Parsed entries are runtime-only views rebuilt from the raw tail.
// After a successful load any block already fully covered by resident spans is
// handed to writeback.
func (e *PackReader) ensureIndexLoaded(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var loaded, closed, started bool
	var waitCh chan struct{}
	var cache IndexCache
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			closed = true
			return
		}
		if e.indexLoaded {
			loaded = true
			return
		}
		if e.indexLoadCh != nil {
			waitCh = e.indexLoadCh
			return
		}
		e.indexLoadCh = make(chan struct{})
		waitCh = e.indexLoadCh
		cache = e.indexCache
		e.workCount++
		started = true
	})
	if closed {
		return context.Canceled
	}
	if loaded {
		return nil
	}
	if started {
		e.startIndexLoad(cache)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.ctx.Done():
		return context.Canceled
	case <-waitCh:
	}
	var err error
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = e.indexLoadErr
	})
	return err
}

// startIndexLoad loads and publishes the pack index under the PackReader lifetime.
func (e *PackReader) startIndexLoad(cache IndexCache) {
	startOwnerWork(func() {
		defer e.finishOwnerWork()

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

		var writeback func()
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			loadCh := e.indexLoadCh
			if e.closed {
				err = context.Canceled
			}
			if err == nil {
				e.setIndexEntriesLocked(entries)
				writeback = e.prepareWritebackLocked(0, e.size)
				e.indexLoaded = true
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
	})
}

func (e *PackReader) snapshotFetchedBytes() int64 {
	var bytes int64
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		bytes = e.fetchBytes
	})
	return bytes
}

func (e *PackReader) recordIndexCacheHit() {
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheHits++
		notify = e.statsChanged
		broadcast()
	})
	if notify != nil {
		notify()
	}
}

func (e *PackReader) recordIndexCacheMiss() {
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheMisses++
		notify = e.statsChanged
		broadcast()
	})
	if notify != nil {
		notify()
	}
}

func (e *PackReader) recordIndexCacheReadError() {
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheReadErrors++
		notify = e.statsChanged
		broadcast()
	})
	if notify != nil {
		notify()
	}
}

func (e *PackReader) recordIndexCacheWriteError() {
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.indexCacheWriteErrors++
		notify = e.statsChanged
		broadcast()
	})
	if notify != nil {
		notify()
	}
}

func (e *PackReader) recordRemoteIndexLoad(bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	var notify func()
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.remoteIndexLoads++
		e.remoteIndexBytes += bytes
		e.lastRemoteIndexBytes = bytes
		notify = e.statsChanged
		broadcast()
	})
	if notify != nil {
		notify()
	}
}

// readIndexTailEntries reads the raw kvfile index tail and returns parsed entries.
//
// Tail reads go through the engine's ReaderAt, so the bytes land in the shared
// span store. Blocks fully contained in those spans can be written back
// without another network round trip.
func (e *PackReader) readIndexTailEntries(ctx context.Context) ([]byte, []*kvfile.IndexEntry, error) {
	if e.size < 0 {
		return nil, nil, errors.Errorf("negative pack size %d", e.size)
	}
	tail, err := e.readIndexTailSuffix(ctx, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "read kvfile index tail")
	}
	entries, err := e.parseIndexTail(tail)
	if err != nil {
		maxTail, maxErr := e.readIndexTailSuffix(ctx, true)
		if maxErr != nil {
			return nil, nil, err
		}
		if len(maxTail) != len(tail) {
			tail = maxTail
			entries, err = e.parseIndexTail(tail)
		}
	}
	return tail, entries, err
}

func (e *PackReader) readIndexTailSuffix(ctx context.Context, maxBound bool) ([]byte, error) {
	window, err := e.indexTailWindow(maxBound)
	if err != nil {
		return nil, err
	}
	if window <= 0 {
		return nil, errors.New("index tail window is empty")
	}
	start := max(e.size-int64(window), 0)
	if err := e.ensureResident(ctx, start, e.size, true); err != nil {
		return nil, err
	}
	suffix, ok := e.readResidentRange(start, e.size)
	if !ok {
		return nil, ErrIncompleteCachedPackRange
	}
	_, tail, err := kvfile.TrimIndexTail(suffix, uint64(e.size)) //nolint:gosec // e.size is rejected when negative and is the validated pack length.
	if err != nil {
		return nil, err
	}
	return tail, nil
}

func (e *PackReader) indexTailWindow(maxBound bool) (int, error) {
	maxTail, err := kvfile.MaxIndexTailSize(e.blockCount)
	if err != nil {
		return 0, err
	}
	if maxTail > uint64(e.size) { //nolint:gosec // e.size is non-negative and comes from the validated pack reader.
		maxTail = uint64(e.size) //nolint:gosec // e.size is non-negative and comes from the validated pack reader.
	}
	if maxBound {
		return int(maxTail), nil //nolint:gosec // maxTail is capped by the non-negative int64 pack size.
	}
	window := uint64(defaultIndexTailInitialWindow)
	estimated := 8 + e.blockCount*(128+8+10)
	if estimated > window {
		window = estimated
	}
	if window > maxTail {
		window = maxTail
	}
	if window > uint64(e.size) { //nolint:gosec // e.size is non-negative and comes from the validated pack reader.
		window = uint64(e.size) //nolint:gosec // e.size is non-negative and comes from the validated pack reader.
	}
	return int(window), nil //nolint:gosec // window is capped by the non-negative int64 pack size.
}

func (e *PackReader) parseIndexTail(tail []byte) ([]*kvfile.IndexEntry, error) {
	if e.size < 0 {
		return nil, errors.Errorf("negative pack size %d", e.size)
	}
	reader, err := kvfile.BuildReaderWithIndexTail(tail, uint64(e.size))
	if err != nil {
		return nil, errors.Wrap(err, "build kvfile reader from index tail")
	}
	if reader.Size() != e.blockCount {
		return nil, errors.Errorf("index entry count %d != manifest block count %d", reader.Size(), e.blockCount)
	}
	count := reader.Size()
	if count > uint64(math.MaxInt) {
		return nil, errors.Errorf("index entry count %d overflows int", count)
	}
	entries := make([]*kvfile.IndexEntry, 0, count)
	err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		entries = append(entries, ie.CloneVT())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan index entries")
	}
	tailStart := uint64(e.size) - uint64(len(tail))
	if err := validateIndexEntries(entries, tailStart, e.blockCount); err != nil {
		return nil, err
	}
	return entries, nil
}

func validateIndexEntries(entries []*kvfile.IndexEntry, tailStart uint64, blockCount uint64) error {
	if uint64(len(entries)) != blockCount {
		return errors.Errorf("index entry count %d != manifest block count %d", len(entries), blockCount)
	}
	var prev []byte
	for i, entry := range entries {
		if entry == nil {
			return errors.Errorf("nil index entry at %d", i)
		}
		key := entry.GetKey()
		if len(key) == 0 {
			return errors.Errorf("empty index key at %d", i)
		}
		if prev != nil && bytes.Compare(prev, key) >= 0 {
			return errors.Errorf("duplicate or unsorted index key at %d", i)
		}
		prev = key
		if _, err := parseBlockRef(entry); err != nil {
			return errors.Wrapf(err, "parse index key at %d", i)
		}
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

// setIndexEntriesLocked stores the index entries sorted by offset and by key.
func (e *PackReader) setIndexEntriesLocked(entries []*kvfile.IndexEntry) {
	byOff := slices.Clone(entries)
	slices.SortFunc(byOff, func(a, b *kvfile.IndexEntry) int {
		return cmp.Compare(a.GetOffset(), b.GetOffset())
	})
	byKey := slices.Clone(entries)
	slices.SortFunc(byKey, func(a, b *kvfile.IndexEntry) int {
		return bytes.Compare(a.GetKey(), b.GetKey())
	})
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
	targetOff, targetEnd := entryExtent(target)
	start := targetOff
	end := targetEnd
	if e.writebackTarget == nil || e.writebackWindow <= 0 {
		return start, end
	}

	half := e.writebackWindow / 2
	intendedStart := int64(0)
	if targetOff > half {
		intendedStart = targetOff - half
	}
	intendedEnd := targetEnd + half

	pos := sort.Search(len(e.entriesByOff), func(i int) bool {
		return int64(e.entriesByOff[i].GetOffset()) >= intendedStart //nolint:gosec // validateIndexEntries bounds offsets by the int64 pack size.
	})
	for ; pos < len(e.entriesByOff); pos++ {
		eOff, eEnd := entryExtent(e.entriesByOff[pos])
		if eOff >= intendedEnd {
			break
		}
		if eEnd > intendedEnd {
			continue
		}
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
