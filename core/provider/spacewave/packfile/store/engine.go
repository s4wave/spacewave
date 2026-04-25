package store

import (
	"bytes"
	"context"
	"io"
	"math"
	"runtime"
	"sort"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/conc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// Default tuning knobs for a pack access engine. See the packfile-reader
// rewrite design doc for rationale.
const (
	// defaultTransportMinWindow is the minimum transport fetch size and the
	// alignment quantum. Large enough to amortize Cloudflare/R2 overhead.
	defaultTransportMinWindow = 1 * 1024 * 1024
	// defaultTransportMaxWindow caps any single transport window.
	defaultTransportMaxWindow = 128 * 1024 * 1024
	// defaultTransportPageBytes is the internal page size for resident spans.
	defaultTransportPageBytes = 4 * 1024
	// defaultTransportTargetHz is the steady-state request-rate target.
	defaultTransportTargetHz = 4.0
	// defaultTransportWindowSmoothing is the weight for upward window growth.
	defaultTransportWindowSmoothing = 0.25
	// defaultSparseColdWindow is the first sparse-read payload window.
	defaultSparseColdWindow = 128 * 1024
	// defaultSparseLocalityDistance is the distance that promotes sparse
	// reads into the normal adaptive window path.
	defaultSparseLocalityDistance = 512 * 1024
	// defaultResidentBudget is the default resident-byte budget per engine.
	defaultResidentBudget = 256 * 1024 * 1024
	// defaultWritebackWindow is the default semantic co-block window.
	defaultWritebackWindow = 128 * 1024
)

// defaultVerifyConcurrency returns the default verify/persist worker count.
func defaultVerifyConcurrency() int {
	n := runtime.GOMAXPROCS(0)
	if n <= 0 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

// PackReader is the per-pack access engine.
//
// The engine composes three conceptual projections over one shared byte
// substrate:
//
//   - Span Store: resident raw packfile bytes, LRU eviction, uncovered-gap
//     planning for transport fetches.
//   - Block Catalog: physical-order map of known block extents with
//     per-block publication state (loaded, verifying, verified, failed).
//   - Publication Queue: verify-hash + optional writeback, keyed by block
//     identity so overlapping fetches never duplicate work.
//
// The engine is addressable as an io.ReaderAt (backed by the span store) and
// exposes higher-level GetBlock operations keyed by kvfile index entries.
type PackReader struct {
	// Immutable after construction.
	packID     string
	size       int64
	transport  Transport
	hashType   hash.HashType
	blockCount uint64

	// bcast guards all mutable state below.
	bcast broadcast.Broadcast

	// Tuning (mutable via setters, guarded by bcast).
	pageSize               int
	minWindow              int
	transportQuantum       int
	maxWindow              int
	currentWindow          int
	smoothing              float64
	targetInterval         time.Duration
	sparseReads            bool
	sparseColdWindow       int
	sparseLocalityDistance int64
	maxBytes               int64
	writebackWindow        int64
	indexPromotion         bool

	// Span store.
	spans         []*span
	residentBytes int64
	useSeq        uint64
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

	// Block catalog.
	blocks                map[string]*blockRecord
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

	// Publication / writeback / verify.
	indexCache      IndexCache
	writebackCtx    context.Context
	writebackTarget block.StoreOps
	verifyQueue     *conc.ConcurrentQueue
	ownVerifyQueue  bool
	verifyQueued    int
	verifyRunning   int
	verifyCompleted uint64
	verifyFailures  uint64
	writebackCount  uint64
	writebackErrors uint64
	lastPublishDur  time.Duration
	statsChanged    func()
}

// NewPackReader builds a per-pack access engine wrapping a transport.
func NewPackReader(packID string, size int64, transport Transport, hashType hash.HashType) *PackReader {
	return &PackReader{
		packID:                 packID,
		size:                   size,
		transport:              transport,
		hashType:               hashType,
		pageSize:               defaultTransportPageBytes,
		minWindow:              defaultTransportMinWindow,
		transportQuantum:       defaultTransportMinWindow,
		maxWindow:              defaultTransportMaxWindow,
		currentWindow:          defaultTransportMinWindow,
		smoothing:              defaultTransportWindowSmoothing,
		targetInterval:         time.Duration(float64(time.Second) / defaultTransportTargetHz),
		sparseReads:            true,
		sparseColdWindow:       defaultSparseColdWindow,
		sparseLocalityDistance: defaultSparseLocalityDistance,
		maxBytes:               defaultResidentBudget,
		writebackWindow:        defaultWritebackWindow,
		indexPromotion:         true,
		blocks:                 make(map[string]*blockRecord),
	}
}

// SetMaxBytes sets the resident byte budget.
func (e *PackReader) SetMaxBytes(maxBytes int64) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.maxBytes = maxBytes
		e.evictLocked()
		broadcast()
	})
}

// SetWriteback configures co-block publication to a target store.
//
// ctx scopes background publication work. target receives verified block
// copies when non-nil. windowBytes is the semantic neighborhood window for
// eager co-block verification around a miss. Pass 0 to use the default.
func (e *PackReader) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	if windowBytes <= 0 {
		windowBytes = defaultWritebackWindow
	}
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
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
		e.indexCache = cache
	})
}

// SetVerifyQueue shares a verify/persist worker pool with the engine.
//
// Callers may share one queue across multiple engines to bound total verify
// concurrency. When not set, the engine lazily creates its own pool at the
// default concurrency the first time work is enqueued.
func (e *PackReader) SetVerifyQueue(q *conc.ConcurrentQueue) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.verifyQueue = q
		e.ownVerifyQueue = false
	})
}

// SetStatsChangedCallback sets a callback invoked after observable stats change.
func (e *PackReader) SetStatsChangedCallback(fn func()) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
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

// enqueueVerifyLocked submits verify/publish jobs to the shared worker pool.
// Must be called with bcast held.
func (e *PackReader) enqueueVerifyLocked(jobs ...func()) {
	if len(jobs) == 0 {
		return
	}
	if e.verifyQueue == nil {
		e.verifyQueue = conc.NewConcurrentQueue(defaultVerifyConcurrency())
		e.ownVerifyQueue = true
	}
	e.verifyQueued += len(jobs)
	wrapped := make([]func(), 0, len(jobs))
	for _, job := range jobs {
		if job == nil {
			e.verifyQueued--
			continue
		}
		wrapped = append(wrapped, e.wrapVerifyJob(job))
	}
	if len(wrapped) == 0 {
		return
	}
	e.verifyQueue.Enqueue(wrapped...)
}

func (e *PackReader) wrapVerifyJob(job func()) func() {
	return func() {
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if e.verifyQueued > 0 {
				e.verifyQueued--
			}
			e.verifyRunning++
			broadcast()
		})
		defer func() {
			e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if e.verifyRunning > 0 {
					e.verifyRunning--
				}
				e.verifyCompleted++
				broadcast()
			})
		}()
		job()
	}
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
		if err := r.e.fetchMiss(r.ctx, cur, end); err != nil {
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
		return compareKey(entries[i].GetKey(), key) >= 0
	})
	if i < len(entries) && compareKey(entries[i].GetKey(), key) == 0 {
		return entries[i], true
	}
	return nil, false
}

// compareKey is bytes.Compare on two byte keys.
func compareKey(a, b []byte) int {
	return bytes.Compare(a, b)
}

// _ is a type assertion
var _ io.ReaderAt = (*engineReaderAt)(nil)
