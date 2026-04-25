package store

import (
	"context"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// blockState is the publication state of a block record.
type blockState int

const defaultIndexTailInitialWindow = 256 * 1024

const (
	// blockStateLoaded indicates the bytes are resident but not yet verified.
	blockStateLoaded blockState = iota
	// blockStateVerifying indicates hash verification is scheduled or running.
	blockStateVerifying
	// blockStateVerified indicates hash verification succeeded.
	blockStateVerified
	// blockStatePublished indicates the block was written to the writeback target.
	blockStatePublished
	// blockStateFailed indicates hash verification failed.
	blockStateFailed
)

// blockRecord is the publication state of one known block extent.
//
// Block records replace the legacy "logical range cache" as the semantic
// object the engine cares about. Each record points at the subset of spans
// that cover its physical extent and pins them so block bytes remain
// reachable across range evictions.
type blockRecord struct {
	// key is the kvfile index key (block hash as base58 string).
	key string
	// ref is the parsed block ref.
	ref *block.BlockRef
	// off is the absolute packfile offset of the block data.
	off int64
	// size is the block data size in bytes.
	size int64
	// spans is the subset of resident spans fully covering [off, off+size).
	spans []*span
	// state is the current publication state.
	state blockState
	// err is the verify/publish error, if any.
	err error
	// readyCh closes when verification finishes (success or failure).
	readyCh chan struct{}
	// queued indicates verify work has been enqueued.
	queued bool
	// writtenBack indicates the block was successfully published.
	writtenBack bool
	// isTarget indicates this was the original fetch target (for tests).
	isTarget bool
	// lastUseSeq is the LRU sequence.
	lastUseSeq uint64
	// enqueueAt is when publication work was first queued.
	enqueueAt time.Time
}

// readBytes copies the block bytes out of the backing spans.
func (b *blockRecord) readBytes() ([]byte, error) {
	out := make([]byte, b.size)
	if copySpans(out, b.spans, b.off) != len(out) {
		return nil, errors.Wrapf(
			ErrIncompleteCachedPackRange,
			"pack-block key=%s off=%d size=%d",
			b.key, b.off, b.size,
		)
	}
	return out, nil
}

// ensureIndexLoaded loads the kvfile index for this pack if not already loaded.
//
// The raw index-tail cache is consulted first. On a miss, the engine slices
// the kvfile tail through its own ReaderAt so tail bytes land in the shared
// span store. Parsed entries are runtime-only views rebuilt from the raw tail.
// After a successful load any block already fully covered by resident spans is
// promoted into the block catalog and enqueued for verification.
func (e *PackReader) ensureIndexLoaded(ctx context.Context) error {
	var leader bool
	var waitCh chan struct{}
	var cache IndexCache
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.indexLoaded {
			return
		}
		if e.indexLoadCh != nil {
			waitCh = e.indexLoadCh
			return
		}
		e.indexLoadCh = make(chan struct{})
		leader = true
		cache = e.indexCache
	})

	if !leader {
		if waitCh == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
		var err error
		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			err = e.indexLoadErr
		})
		return err
	}

	var tail []byte
	var entries []*kvfile.IndexEntry
	var err error
	if cache != nil {
		cached, ok, cacheErr := cache.Get(ctx, e.packID)
		if cacheErr != nil {
			e.recordIndexCacheReadError()
		} else if ok {
			entries, cacheErr = e.parseIndexTail(cached)
			if cacheErr != nil {
				e.recordIndexCacheReadError()
			}
			if cacheErr == nil {
				e.recordIndexCacheHit()
				tail = cached
			}
		} else {
			e.recordIndexCacheMiss()
		}
	}
	if entries == nil {
		before := e.snapshotFetchedBytes()
		tail, entries, err = e.readIndexTailEntries(ctx)
		e.recordRemoteIndexLoad(e.snapshotFetchedBytes() - before)
		if err == nil && cache != nil {
			if cacheErr := cache.Set(ctx, e.packID, tail); cacheErr != nil {
				e.recordIndexCacheWriteError()
			}
		}
	}

	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		loadCh := e.indexLoadCh
		if err == nil {
			e.setIndexEntriesLocked(entries)
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
	return err
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
// This is the trailer-promotion path: kvfile tail reads go through the engine's
// ReaderAt, which means bytes land in the shared span store. After this call
// returns, any block fully contained within those spans can be promoted without
// an extra network round trip.
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
	if err := e.ensureExactRangeResident(ctx, start, e.size); err != nil {
		return nil, err
	}
	suffix, ok := e.readResidentRange(start, e.size)
	if !ok {
		return nil, ErrIncompleteCachedPackRange
	}
	_, tail, err := kvfile.TrimIndexTail(suffix, uint64(e.size))
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
	if maxTail > uint64(e.size) {
		maxTail = uint64(e.size)
	}
	if maxBound {
		return int(maxTail), nil
	}
	window := uint64(defaultIndexTailInitialWindow)
	estimated := 8 + e.blockCount*(128+8+10)
	if estimated > window {
		window = estimated
	}
	if window > maxTail {
		window = maxTail
	}
	if window > uint64(e.size) {
		window = uint64(e.size)
	}
	return int(window), nil
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
		if prev != nil && compareKey(prev, key) >= 0 {
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

// setIndexEntriesLocked sorts and stores index entries. Must be called with bcast held.
func (e *PackReader) setIndexEntriesLocked(entries []*kvfile.IndexEntry) {
	byOff := slices.Clone(entries)
	sort.Slice(byOff, func(i, j int) bool {
		return byOff[i].GetOffset() < byOff[j].GetOffset()
	})
	byKey := slices.Clone(entries)
	sort.Slice(byKey, func(i, j int) bool {
		return compareKey(byKey[i].GetKey(), byKey[j].GetKey()) < 0
	})
	e.entriesByOff = byOff
	e.entriesByKey = byKey

	// Promote any blocks already fully covered by the spans we fetched
	// during the trailer/index read (or from earlier block fetches).
	for _, sp := range e.spans {
		e.promoteBlocksInSpanLocked(sp)
	}
}

// promoteBlocksInSpanLocked registers loaded block records for every index
// entry fully contained in the given span.
//
// Requires the span to already be inserted in the span store. Enqueues a
// verify job for each new record so publication flows through the same
// pipeline used by regular block fetches.
func (e *PackReader) promoteBlocksInSpanLocked(sp *span) {
	if sp == nil || len(e.entriesByOff) == 0 || !e.indexPromotion {
		return
	}
	pos := sort.Search(len(e.entriesByOff), func(i int) bool {
		return int64(e.entriesByOff[i].GetOffset()) >= sp.off
	})
	var jobs []func()
	for ; pos < len(e.entriesByOff); pos++ {
		entry := e.entriesByOff[pos]
		eOff := int64(entry.GetOffset())
		if eOff >= sp.end() {
			break
		}
		eEnd := eOff + int64(entry.GetSize())
		if eEnd > sp.end() {
			continue
		}
		job, ok := e.admitBlockLocked(entry, eOff, eEnd, false)
		if ok && job != nil {
			jobs = append(jobs, job)
		}
	}
	if len(jobs) != 0 {
		e.enqueueVerifyLocked(jobs...)
	}
}

// admitBlockLocked creates or touches a block record covering [off, end).
//
// Requires at least one resident span to cover that interval. Returns the
// (possibly nil) verify job to enqueue. A job is returned only when a new
// record was created; existing records are left alone to avoid duplicate
// verification.
func (e *PackReader) admitBlockLocked(entry *kvfile.IndexEntry, off, end int64, isTarget bool) (func(), bool) {
	key := string(entry.GetKey())
	if existing, ok := e.blocks[key]; ok {
		e.useSeq++
		existing.lastUseSeq = e.useSeq
		return nil, true
	}
	spans, covered := e.collectSpansLocked(off, end)
	if !covered {
		return nil, false
	}
	ref, err := parseBlockRef(entry)
	if err != nil {
		return nil, false
	}
	rec := &blockRecord{
		key:       key,
		ref:       ref,
		off:       off,
		size:      end - off,
		spans:     spans,
		state:     blockStateVerifying,
		readyCh:   make(chan struct{}),
		queued:    true,
		isTarget:  isTarget,
		enqueueAt: time.Now(),
	}
	e.retainSpansLocked(spans)
	e.useSeq++
	rec.lastUseSeq = e.useSeq
	e.blocks[key] = rec
	return func() { e.verifyBlock(rec) }, true
}

// lookupBlockLocked returns any existing block record by key.
func (e *PackReader) lookupBlockLocked(key string) *blockRecord {
	rec := e.blocks[key]
	if rec != nil {
		e.useSeq++
		rec.lastUseSeq = e.useSeq
	}
	return rec
}

// removeBlockLocked removes a block record and releases its span pins.
func (e *PackReader) removeBlockLocked(rec *blockRecord) {
	if _, ok := e.blocks[rec.key]; !ok {
		return
	}
	delete(e.blocks, rec.key)
	e.releaseSpansLocked(rec.spans)
	rec.spans = nil
}

// releasableBytesLocked returns the bytes that would become unpinned if rec
// were removed.
func (e *PackReader) releasableBytesLocked(rec *blockRecord) int64 {
	if rec == nil {
		return 0
	}
	var releasable int64
	for _, sp := range rec.spans {
		if sp == nil {
			continue
		}
		if sp.pins == 1 {
			releasable += sp.size
		}
	}
	return releasable
}

// pickEvictionRecordLocked chooses a completed block record to unpin under
// resident-byte pressure.
func (e *PackReader) pickEvictionRecordLocked() *blockRecord {
	overage := e.residentBytes - e.maxBytes
	if overage <= 0 {
		return nil
	}

	var best *blockRecord
	var bestFree int64
	for _, rec := range e.blocks {
		if rec == nil || rec.queued {
			continue
		}
		if rec.state != blockStateVerified && rec.state != blockStatePublished {
			continue
		}
		free := e.releasableBytesLocked(rec)
		if free == 0 {
			continue
		}
		bestSufficient := bestFree >= overage
		freeSufficient := free >= overage
		if best == nil ||
			(!bestSufficient && freeSufficient) ||
			(bestSufficient == freeSufficient && bestSufficient && free < bestFree) ||
			(bestSufficient == freeSufficient && !bestSufficient && free > bestFree) ||
			(bestSufficient == freeSufficient && free == bestFree && rec.lastUseSeq < best.lastUseSeq) {
			best = rec
			bestFree = free
		}
	}
	return best
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
// target bytes. The returned slice of contained entries always includes the
// target.
func (e *PackReader) semanticWindowLocked(target *kvfile.IndexEntry) (int64, int64, []*kvfile.IndexEntry) {
	targetOff := int64(target.GetOffset())
	targetEnd := targetOff + int64(target.GetSize())

	start := targetOff
	end := targetEnd
	contained := []*kvfile.IndexEntry{target}

	if e.writebackTarget == nil || e.writebackWindow <= 0 {
		return start, end, contained
	}

	half := e.writebackWindow / 2
	intendedStart := int64(0)
	if targetOff > half {
		intendedStart = targetOff - half
	}
	intendedEnd := targetEnd + half

	pos := sort.Search(len(e.entriesByOff), func(i int) bool {
		return int64(e.entriesByOff[i].GetOffset()) >= intendedStart
	})
	for ; pos < len(e.entriesByOff); pos++ {
		entry := e.entriesByOff[pos]
		eOff := int64(entry.GetOffset())
		eEnd := eOff + int64(entry.GetSize())
		if eOff >= intendedEnd {
			break
		}
		if entry == target || eEnd > intendedEnd {
			continue
		}
		if eOff < start {
			start = eOff
		}
		if eEnd > end {
			end = eEnd
		}
		contained = append(contained, entry)
	}
	return start, end, contained
}

// ensureWindowResident ensures the byte window [start, end) is resident,
// fetching uncovered sub-intervals via the span store planner.
func (e *PackReader) ensureWindowResident(ctx context.Context, start, end int64) error {
	if end <= start {
		return nil
	}
	for cur := start; cur < end; {
		var resident *span
		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			resident = e.findCoveringSpanLocked(cur)
		})
		if resident != nil {
			cur = min(end, resident.end())
			continue
		}
		if err := e.fetchMiss(ctx, cur, end); err != nil {
			return err
		}
	}
	return nil
}

// parseBlockRef builds a block ref from a kvfile index entry key.
func parseBlockRef(entry *kvfile.IndexEntry) (*block.BlockRef, error) {
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(entry.GetKey())); err != nil {
		return nil, err
	}
	return &block.BlockRef{Hash: h}, nil
}
