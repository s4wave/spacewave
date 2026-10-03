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
		// Fence reader work and cancel the admitted jobs once.
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
		// Wait for admitted reader jobs before releasing their shared state.
		var waitCh <-chan struct{}
		e.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// Retain reader dependencies until all admitted jobs finish.
			if e.workCount != 0 {
				waitCh = getWaitCh()
				return
			}

			// Release transport, index, and writeback dependencies.
			e.transport = nil
			e.indexCache = nil
			e.writebackCtx = nil
			e.writebackTarget = nil
			e.entriesByOff = nil
			e.entriesByKey = nil
			e.published = nil

			// Release resident spans and return their bytes to the shared budget.
			e.spans = nil
			e.lru.Init()
			e.newest = nil
			e.chargeLocked(-e.residentBytes)
			e.budget.detach(e)

			// Release fetch state and publish reader close completion.
			e.loading = nil
			e.statsChanged = nil
			e.closeComplete = true
			broadcast()
		})

		// Finish once the reader dependencies have been released.
		if waitCh == nil {
			return
		}

		// Wait for the reader work or close state to change.
		<-waitCh
	}
}

// waitCloseComplete waits for the first Close caller to drain reader work.
func (e *PackReader) waitCloseComplete() {
	for {
		// Read reader close completion together with its next notification.
		var complete bool
		var waitCh <-chan struct{}
		e.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			complete = e.closeComplete
			if !complete {
				waitCh = getWaitCh()
			}
		})

		// Finish after the first close caller has drained the reader.
		if complete {
			return
		}

		// Wait for the reader work or close state to change.
		<-waitCh
	}
}

// finishOwnerWork releases one admitted job and wakes Close if it is waiting.
func (e *PackReader) finishOwnerWork() {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		e.workCount--
		broadcast()
	})
}

// setBudget moves the reader's resident bytes onto a shared budget.
func (e *PackReader) setBudget(budget *residentBudget) {
	// Transfer resident byte accounting to the shared reader budget.
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Move resident accounting only for an open reader on a different budget.
		if e.closed || e.budget == budget {
			return
		}
		e.budget.detach(e)
		e.budget.used.Add(-e.residentBytes)
		e.budget = budget
		e.budget.used.Add(e.residentBytes)
		e.budget.attach(e)
	})

	// Reclaim resident spans above the shared byte limit.
	budget.reclaim()
}

// SetWriteback configures co-block publication to a target store.
//
// ctx scopes background writeback work. target receives verified block
// copies when non-nil. windowBytes is the neighborhood fetched around a miss
// so its co-blocks are published with it. Pass 0 to use the default.
func (e *PackReader) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	// Choose the default neighbor window for pack writeback.
	if windowBytes <= 0 {
		windowBytes = defaultWritebackWindow
	}

	// Publish writeback configuration while the reader remains open.
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
	// ctx bounds waits for this caller without canceling shared fetches.
	ctx context.Context
	// e holds the shared pack cache and transport lifetime.
	e *PackReader
}

// ReadAt implements io.ReaderAt by routing through the span store.
func (r *engineReaderAt) ReadAt(p []byte, off int64) (int, error) {
	// Bound the requested interval by the pack extent.
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

	// Copy each immutable fetch result before advancing, even after cache eviction.
	n := 0
	for cur := off; cur < end; {
		sp, err := r.e.fetchRange(r.ctx, cur, end, false)
		if err != nil {
			return n, err
		}
		nread := sp.readAt(p[n:n+int(end-cur)], cur)
		n += nread
		cur += int64(nread)
	}

	// Report a short read when the request extended past the pack.
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// fetchKey identifies one in-flight transport fetch.
type fetchKey struct {
	// off is the first requested pack byte.
	off int64
	// size is the requested byte count.
	size int
}

// end returns the exclusive end of the requested interval.
func (k fetchKey) end() int64 { return k.off + int64(k.size) }

// fetchLoad tracks one in-flight transport fetch.
type fetchLoad struct {
	// done publishes sp and err to every waiter when the fetch finishes.
	done chan struct{}
	// sp retains the immutable response independently of cache residency.
	sp *span
	// err is the transport or reader shutdown error.
	err error
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
	// Clamp the transport window to the configured byte limits.
	if size < e.minWindow {
		size = e.minWindow
	}
	if e.maxWindow > 0 && size > e.maxWindow {
		size = e.maxWindow
	}

	// Align the transport window to its configured fetch quantum.
	quantum := int64(max(1, e.transportQuantum))
	return int(alignUp(int64(size), quantum))
}

// smoothWindow smooths upward window growth.
func (e *PackReader) smoothWindow(current, target int) int {
	// Apply a smaller transport window without smoothing.
	if target <= current {
		return target
	}

	// Smooth upward transport growth within the configured window bounds.
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
