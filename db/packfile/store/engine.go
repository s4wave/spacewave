package store

import (
	"bytes"
	"container/list"
	"context"
	"io"
	"math"
	"sort"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
)

// Default tuning knobs for a pack access engine. See the packfile-reader
// rewrite design doc for rationale.
const (
	// defaultTransportMinWindow is the minimum transport fetch size and the
	// alignment quantum. Large enough to amortize Cloudflare/R2 overhead.
	defaultTransportMinWindow = 1 * 1024 * 1024
	// defaultTransportMaxWindow caps any single transport window.
	defaultTransportMaxWindow = 128 * 1024 * 1024
	// defaultTransportTargetHz is the steady-state request-rate target.
	defaultTransportTargetHz = 4.0
	// defaultTransportWindowSmoothing is the weight for upward window growth.
	defaultTransportWindowSmoothing = 0.25
	// defaultSparseColdWindow is the first sparse-read payload window.
	defaultSparseColdWindow = 128 * 1024
	// defaultSparseLocalityDistance is the distance that promotes sparse
	// reads into the normal adaptive window path.
	defaultSparseLocalityDistance = 512 * 1024
	// defaultResidentBudget is the default resident-byte budget of a store,
	// or of a reader opened outside a store.
	defaultResidentBudget = 256 * 1024 * 1024
	// defaultWritebackWindow is the default semantic co-block window.
	defaultWritebackWindow = 128 * 1024
)

// PackReader is the per-pack access engine.
//
// The engine composes two projections over one shared byte substrate:
//
//   - Span Store: resident raw packfile bytes, LRU eviction, uncovered-gap
//     planning for transport fetches.
//   - Block Index: the kvfile index entries of the pack, and the set of
//     blocks handed to the writeback target.
//
// Reads verify the requested block inline. Each fetched span hands every
// block it completes to one batched writeback job, so a block is published
// at most once.
//
// The engine is addressable as an io.ReaderAt (backed by the span store) and
// exposes higher-level GetBlock operations keyed by kvfile index entries.
type PackReader struct {
	// ctx is canceled when Close begins draining admitted work.
	ctx context.Context
	// cancel ends ctx during Close.
	cancel context.CancelFunc
	// packID identifies the pack served by this reader.
	packID string
	// size is the immutable packfile size.
	size int64
	// transport fetches uncovered packfile ranges.
	transport Transport
	// blockCount validates the loaded pack index.
	blockCount uint64

	// bcast guards all mutable state below.
	bcast broadcast.Broadcast
	// closed rejects work after Close begins draining it.
	closed bool
	// closeComplete records that Close has released all reader state.
	closeComplete bool
	// workCount is the number of admitted index, fetch, and writeback jobs.
	workCount int

	// Tuning (mutable via setters, guarded by bcast).
	minWindow              int
	transportQuantum       int
	maxWindow              int
	transportFetchMaxBytes int
	currentWindow          int
	smoothing              float64
	targetInterval         time.Duration
	sparseReads            bool
	sparseColdWindow       int
	sparseLocalityDistance int64
	writebackWindow        int64

	// Span store. spans are sorted by offset and disjoint. lru orders the
	// spans from least to most recently used. newest is the span inserted
	// last, which eviction skips until a reader can consume it.
	spans         []*span
	lru           list.List
	newest        *span
	residentBytes int64
	budget        *residentBudget
	loading       map[fetchKey]*fetchLoad

	// Adaptive window tracking.
	lastFetchAt            time.Time
	lastFetchBytes         int
	lastTargetOff          int64
	lastTargetEnd          int64
	lastTargetSet          bool
	fetchCount             uint64
	fetchBytes             int64
	rangeResponseBytes     int64
	indexTailFetchCount    uint64
	indexTailFetchBytes    int64
	indexTailResponseBytes int64

	// Block index. published holds the keys handed to the writeback target.
	published             map[string]struct{}
	entriesByOff          []*kvfile.IndexEntry
	entriesByKey          []*kvfile.IndexEntry
	indexLoaded           bool
	indexLoadCh           chan struct{}
	indexLoadErr          error
	indexCacheHits        uint64
	indexCacheMisses      uint64
	indexCacheReadErrors  uint64
	indexCacheWriteErrors uint64
	remoteIndexLoads      uint64
	remoteIndexBytes      int64
	lastRemoteIndexBytes  int64

	// Index cache and writeback.
	indexCache       IndexCache
	writebackCtx     context.Context
	writebackTarget  block.StoreOps
	writebackRunning int
	verifyFailures   uint64
	writebackCount   uint64
	writebackErrors  uint64
	statsChanged     func()
}

// NewPackReader builds a per-pack access engine wrapping a transport.
func NewPackReader(packID string, size int64, transport Transport) *PackReader {
	ctx, cancel := newPackReaderContext()
	e := &PackReader{
		ctx:                    ctx,
		cancel:                 cancel,
		packID:                 packID,
		size:                   size,
		transport:              transport,
		minWindow:              defaultTransportMinWindow,
		transportQuantum:       defaultTransportMinWindow,
		maxWindow:              defaultTransportMaxWindow,
		currentWindow:          defaultTransportMinWindow,
		smoothing:              defaultTransportWindowSmoothing,
		targetInterval:         time.Duration(float64(time.Second) / defaultTransportTargetHz),
		sparseReads:            true,
		sparseColdWindow:       defaultSparseColdWindow,
		sparseLocalityDistance: defaultSparseLocalityDistance,
		writebackWindow:        defaultWritebackWindow,
		budget:                 newResidentBudget(defaultResidentBudget),
		published:              make(map[string]struct{}),
	}
	e.budget.attach(e)
	return e
}

// Close cancels and drains every job admitted by the engine.
func (e *PackReader) Close() {
	// Fence new work and cancel every admitted owner job.
	var closeOwner bool
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		e.closed = true
		e.cancel()
		closeOwner = true
		broadcast()
	})
	if !closeOwner {
		e.waitCloseComplete()
		return
	}

	// Wait for admitted jobs before releasing their reader dependencies.
	for {
		var waitCh <-chan struct{}
		e.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if e.workCount != 0 {
				waitCh = getWaitCh()
				return
			}
			e.transport = nil
			e.indexCache = nil
			e.writebackCtx = nil
			e.writebackTarget = nil
			e.entriesByOff = nil
			e.entriesByKey = nil
			e.published = nil
			e.spans = nil
			e.lru.Init()
			e.newest = nil
			e.chargeLocked(-e.residentBytes)
			e.budget.detach(e)
			e.loading = nil
			e.statsChanged = nil
			e.closeComplete = true
			broadcast()
		})
		if waitCh == nil {
			return
		}
		<-waitCh
	}
}

func (e *PackReader) waitCloseComplete() {
	for {
		var complete bool
		var waitCh <-chan struct{}
		e.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			complete = e.closeComplete
			if !complete {
				waitCh = getWaitCh()
			}
		})
		if complete {
			return
		}
		<-waitCh
	}
}

func (e *PackReader) finishOwnerWork() {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.workCount--
		broadcast()
	})
}

// setBudget moves the reader's resident bytes onto a shared budget.
func (e *PackReader) setBudget(budget *residentBudget) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed || e.budget == budget {
			return
		}
		e.budget.detach(e)
		e.budget.used.Add(-e.residentBytes)
		e.budget = budget
		e.budget.used.Add(e.residentBytes)
		e.budget.attach(e)
	})
	budget.reclaim()
}

// SetWriteback configures co-block publication to a target store.
//
// ctx scopes background writeback work. target receives verified block
// copies when non-nil. windowBytes is the neighborhood fetched around a miss
// so its co-blocks are published with it. Pass 0 to use the default.
func (e *PackReader) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	if windowBytes <= 0 {
		windowBytes = defaultWritebackWindow
	}
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		e.writebackCtx = ctx
		e.writebackTarget = target
		e.writebackWindow = windowBytes
	})
}

// SetExpectedBlockCount configures the manifest block count used to validate index tails.
func (e *PackReader) SetExpectedBlockCount(blockCount uint64) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.blockCount = blockCount
	})
}

// SetIndexCache configures persistent storage for raw kvfile index-tail bytes.
func (e *PackReader) SetIndexCache(cache IndexCache) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		e.indexCache = cache
	})
}

// SetStatsChangedCallback sets a callback invoked after observable stats change.
func (e *PackReader) SetStatsChangedCallback(fn func()) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		e.statsChanged = fn
	})
}

// ReaderAt returns an io.ReaderAt bound to ctx.
//
// Reads issued through the returned ReaderAt fall through the span store:
// resident bytes are served from memory, and misses trigger transport
// fetches via the uncovered-window planner.
func (e *PackReader) ReaderAt(ctx context.Context) io.ReaderAt {
	return &engineReaderAt{ctx: ctx, e: e}
}

// engineReaderAt is a request-scoped io.ReaderAt view onto an engine.
type engineReaderAt struct {
	ctx context.Context
	e   *PackReader
}

// ReadAt implements io.ReaderAt by routing through the span store.
func (r *engineReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.e.size > 0 && off >= r.e.size {
		return 0, io.EOF
	}
	end := off + int64(len(p))
	if r.e.size > 0 && end > r.e.size {
		end = r.e.size
	}
	n := 0
	for cur := off; cur < end; {
		nread := r.e.readFromSpans(p[n:n+int(end-cur)], cur)
		if nread != 0 {
			n += nread
			cur += int64(nread)
			continue
		}
		if err := r.e.fetchRange(r.ctx, cur, end, false); err != nil {
			if n != 0 {
				return n, err
			}
			return 0, err
		}
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// readFromSpans serves bytes from resident spans when possible.
func (e *PackReader) readFromSpans(p []byte, off int64) int {
	var s *span
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s = e.findCoveringSpanLocked(off)
	})
	if s == nil {
		return 0
	}
	return s.readAt(p, off)
}

// fetchKey identifies one in-flight transport fetch.
type fetchKey struct {
	off  int64
	size int
}

func (k fetchKey) end() int64 { return k.off + int64(k.size) }

// fetchLoad tracks one in-flight transport fetch.
type fetchLoad struct {
	done chan struct{}
	sp   *span
	err  error
}

// alignDown rounds v down to the nearest multiple of align.
func alignDown(v, align int64) int64 {
	if align <= 1 {
		return v
	}
	return (v / align) * align
}

// alignUp rounds v up to the nearest multiple of align.
func alignUp(v, align int64) int64 {
	if align <= 1 {
		return v
	}
	return ((v + align - 1) / align) * align
}

// clampWindow clamps size to [minWindow, maxWindow] aligned up to minWindow.
func (e *PackReader) clampWindow(size int) int {
	if size < e.minWindow {
		size = e.minWindow
	}
	if e.maxWindow > 0 && size > e.maxWindow {
		size = e.maxWindow
	}
	quantum := int64(max(1, e.transportQuantum))
	return int(alignUp(int64(size), quantum))
}

// smoothWindow smooths upward window growth.
func (e *PackReader) smoothWindow(current, target int) int {
	if target <= current {
		return target
	}
	smoothed := int(math.Ceil((1-e.smoothing)*float64(current) + e.smoothing*float64(target)))
	return e.clampWindow(smoothed)
}

// binarySearchEntriesByKey returns the index entry matching key in sorted-by-key entries.
func binarySearchEntriesByKey(entries []*kvfile.IndexEntry, key []byte) (*kvfile.IndexEntry, bool) {
	i := sort.Search(len(entries), func(i int) bool {
		return bytes.Compare(entries[i].GetKey(), key) >= 0
	})
	if i < len(entries) && bytes.Equal(entries[i].GetKey(), key) {
		return entries[i], true
	}
	return nil, false
}

// _ is a type assertion
var _ io.ReaderAt = (*engineReaderAt)(nil)
