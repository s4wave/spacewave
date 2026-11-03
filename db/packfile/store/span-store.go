package store

import (
	"context"
	"io"
	"slices"
	"sort"
	"time"

	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// fetchSpans returns immutable spans covering every byte of [start, end).
//
// The returned references keep bytes readable even if later fetches or other
// readers evict them from the resident cache before the caller copies them.
// The planner may widen fetches for read-ahead.
func (e *PackReader) fetchSpans(ctx context.Context, start, end int64) ([]*span, error) {
	// Retain each completed interval before advancing to the next gap.
	var spans []*span
	for cur := start; cur < end; {
		sp, err := e.fetchRange(ctx, cur, end)
		if err != nil {
			return nil, err
		}
		spans = append(spans, sp)
		cur = min(end, sp.end())
	}
	return spans, nil
}

// fetchRange drives a transport fetch to cover off, bounded by readEnd.
//
// It returns an immutable span covering off or an error. The span stays readable
// after cache eviction; callers consume it without a second cache lookup.
// Concurrent callers for overlapping offsets fold onto the same in-flight
// fetch via the loading map, guaranteeing one transport call per uncovered
// span. Transport work belongs to the PackReader, so canceling the caller
// that starts a fetch does not cancel another caller waiting for it.
func (e *PackReader) fetchRange(ctx context.Context, off, readEnd int64) (*span, error) {
	// Record the requested interval and reject canceled demand before fetching.
	ctx, task := trace.NewTask(ctx, "provider/spacewave/packfile/range-fetch")
	defer task.End()
	trace.Log(ctx, "pack-id", e.packID)
	trace.Logf(ctx, "target-offset", "%d", off)
	trace.Logf(ctx, "target-end", "%d", readEnd)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Retain a cached span or join one reader-owned fetch under the state lock.
	var resident *span
	var closed, started bool
	var key fetchKey
	var gate *demandGate
	var promote chan struct{}
	var load *fetchLoad
	var notifyStart func()
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Reject transport demand after the pack reader closes.
		if e.closed {
			closed = true
			return
		}

		// Reuse a resident span before joining transport work.
		resident = e.findCoveringSpanLocked(off)
		if resident != nil {
			return
		}

		// Join a transport fetch that already covers the requested offset. A
		// foreground read promotes a background fetch it joins.
		load = e.findLoadingLocked(off)
		if load != nil {
			if load.promote != nil && !block.IsBackground(ctx) {
				close(load.promote)
				load.promote = nil
			}
			return
		}

		// Plan a transport window within the uncovered gap.
		key = e.planFetchLocked(off, readEnd, block.ReadAhead(ctx))
		if key.size == 0 {
			return
		}

		// Register the reader-owned fetch and retain the reader lifetime.
		if e.loading == nil {
			e.loading = make(map[fetchKey]*fetchLoad)
		}
		load = &fetchLoad{done: make(chan struct{})}
		if block.IsBackground(ctx) {
			load.promote = make(chan struct{})
			promote = load.promote
		}
		gate = e.gate
		e.loading[key] = load
		e.workCount++
		started = true
		notifyStart = e.statsChanged
	})

	// Start transport work outside the lock and preserve its lifetime for waiters.
	if notifyStart != nil {
		notifyStart()
	}
	if closed {
		return nil, ErrPackReaderClosed
	}
	if resident != nil {
		trace.Log(ctx, "result", "resident")
		return resident, nil
	}
	if load == nil {
		trace.Log(ctx, "result", "empty-plan")
		return nil, io.EOF
	}
	if started {
		trace.Log(ctx, "role", "leader")
		trace.Logf(ctx, "range-offset", "%d", key.off)
		trace.Logf(ctx, "range-size", "%d", key.size)
		e.startFetch(key, load, gate, promote)
	}
	if !started {
		trace.Log(ctx, "role", "waiter")
	}

	// The completed load owns its response even when reclaim already evicted it.
	select {
	case <-ctx.Done():
		trace.Log(ctx, "result", "wait-canceled")
		return nil, ctx.Err()
	case <-e.ctx.Done():
		trace.Log(ctx, "result", "owner-canceled")
		return nil, ErrPackReaderClosed
	case <-load.done:
		if load.err != nil {
			trace.Log(ctx, "result", "wait-error")
			return nil, load.err
		}
		if load.sp == nil || off < load.sp.off || off >= load.sp.end() {
			trace.Log(ctx, "result", "wait-empty-response")
			return nil, io.EOF
		}
		trace.Log(ctx, "result", "waited")
		return load.sp, nil
	}
}

// startFetch runs one transport request under the PackReader lifetime. A
// non-nil promote marks a background fetch, which the gate admits after
// foreground fetches unless promote closes first.
func (e *PackReader) startFetch(key fetchKey, load *fetchLoad, gate *demandGate, promote <-chan struct{}) {
	startOwnerWork(func() {
		// Hold the reader open until this job finishes.
		defer e.finishOwnerWork()

		// Fetch the range under the reader's lifetime once the gate admits it.
		var data []byte
		leave, err := gate.enter(e.ctx, promote != nil, promote)
		if err == nil {
			data, err = e.transport.Fetch(e.ctx, key.off, key.size)
			leave()
		}
		var sp *span
		if len(data) != 0 {
			sp = newSpan(key.off, data)
		}

		// Publish the fetched span unless the reader closed during the fetch.
		var notifyDone func()
		var writeback func()
		var budget *residentBudget
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// A closed reader drops the response; otherwise cache the span.
			budget = e.budget
			if e.closed {
				sp = nil
				err = ErrPackReaderClosed
			}
			if err == nil && sp != nil {
				e.insertSpanLocked(sp)
				notifyDone = e.recordFetchLocked(key, len(data))
				writeback = e.prepareWritebackLocked(sp.off, sp.end())
			}
			if notifyDone == nil {
				notifyDone = e.statsChanged
			}
			broadcast()
		})

		// Reclaim resident bytes after publishing the fetched span.
		budget.reclaim()

		// Publish the fetch result and wake every waiting reader.
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Complete the registered fetch and wake its waiting callers.
			load.sp = sp
			load.err = err
			if e.loading[key] == load {
				delete(e.loading, key)
			}
			close(load.done)
			broadcast()
		})

		// Notify stats observers after the fetch completes.
		if notifyDone != nil {
			notifyDone()
		}

		// Persist verified neighbor blocks under the reader lifetime.
		if writeback != nil {
			startOwnerWork(writeback)
		}
	})
}

// planFetchLocked decides what transport window to fetch for a miss at off.
//
// The planner is the only place that translates a miss into a network
// request. It enforces the "never request already-resident bytes" rule: an
// alignment preference may propose a window, but the window is always shifted
// or shrunk inside the uncovered gap around off.
//
// The planner also adapts the steady-state window against the measured
// goodput so the request rate approaches targetInterval^-1 while staying
// clamped to [minWindow, maxWindow].
func (e *PackReader) planFetchLocked(off, readEnd int64, readAhead int) fetchKey {
	// Locate the uncovered byte interval containing the requested offset.
	gapStart, gapEnd, ok := e.findGapLocked(off)
	if !ok || gapEnd <= gapStart {
		return fetchKey{}
	}

	// Determine the required payload extent and its sparse locality.
	mustEnd := min(readEnd, gapEnd)
	if mustEnd <= off {
		mustEnd = off + 1
	}
	needLen := int(max(int64(1), mustEnd-off))
	sparseLocal := false
	if e.sparseReads {
		sparseLocal = e.hasSparseLocalityLocked(off, mustEnd)
	}

	// Adapt the current window against measured goodput: if the last fetch
	// completed in dt, the window that would hit the target request rate is
	// lastBytes * targetInterval / dt. Downward moves apply immediately;
	// upward moves smooth toward the target.
	windowSize := e.currentWindow
	if (!e.sparseReads || sparseLocal) && e.lastFetchBytes > 0 && !e.lastFetchAt.IsZero() {
		dt := time.Since(e.lastFetchAt)
		if dt > 0 && e.targetInterval > 0 {
			target := int(float64(e.lastFetchBytes) * float64(e.targetInterval) / float64(dt))
			target = e.clampWindow(target)
			windowSize = e.smoothWindow(windowSize, target)
			e.currentWindow = windowSize
		}
	}
	if e.sparseReads && !sparseLocal {
		cold := e.clampWindow(e.sparseColdWindow)
		if cold > 0 && windowSize > cold {
			windowSize = cold
		}
	}
	if windowSize < needLen {
		windowSize = e.clampWindow(needLen)
		e.currentWindow = windowSize
	}
	e.recordSparseTargetLocked(off, mustEnd)

	// Bulk-read hints affect this miss only. Keep transport bounds and the
	// shared resident/in-flight gap rules without raising foreground defaults.
	if readAhead > 0 {
		readAhead = e.clampWindow(readAhead)
		if limit := e.budget.limit.Load(); limit > 0 {
			readAhead = int(min(int64(readAhead), limit))
		}
		windowSize = max(windowSize, readAhead)
	}

	// Bound the transport window by the available gap.
	quantum := int64(max(1, e.transportQuantum))
	gapSize := gapEnd - gapStart
	fetchSize := min(int64(windowSize), gapSize)

	// Choose an aligned start containing off.
	start := max(alignDown(off, quantum), gapStart)
	end := max(start+fetchSize, mustEnd)
	if end > gapEnd {
		shift := end - gapEnd
		start -= shift
		end = gapEnd
		start = max(start, gapStart)
	}
	if end <= start {
		return fetchKey{}
	}
	return fetchKey{off: start, size: int(end - start)}
}

// hasSparseLocalityLocked reports proximity to the preceding payload target.
func (e *PackReader) hasSparseLocalityLocked(off, end int64) bool {
	// Require a preceding payload target before detecting sparse locality.
	if !e.lastTargetSet {
		return false
	}
	// Measure the gap between the current and preceding payload targets.
	dist := int64(0)
	if off > e.lastTargetEnd {
		dist = off - e.lastTargetEnd
	} else if e.lastTargetOff > end {
		dist = e.lastTargetOff - end
	}
	return dist <= e.sparseLocalityDistance
}

// recordSparseTargetLocked retains the latest target for locality detection.
func (e *PackReader) recordSparseTargetLocked(off, end int64) {
	// Retain payload targets only while sparse read planning is enabled.
	if !e.sparseReads {
		return
	}

	// Save the payload extent for the next sparse locality decision.
	e.lastTargetOff = off
	e.lastTargetEnd = end
	e.lastTargetSet = true
}

// recordFetchLocked notes a completed fetch for adaptive sizing.
func (e *PackReader) recordFetchLocked(key fetchKey, responseBytes int) func() {
	// Record completed payload transport timing and byte totals.
	e.lastFetchAt = time.Now()
	e.lastFetchBytes = key.size
	e.fetchCount++
	e.fetchBytes += int64(key.size)
	e.rangeResponseBytes += int64(responseBytes)
	return e.statsChanged
}

// recordIndexTailFetchLocked accounts for index I/O separately from payloads.
func (e *PackReader) recordIndexTailFetchLocked(key fetchKey, responseBytes int) func() {
	// Record transport totals for the completed index-tail fetch.
	e.lastFetchAt = time.Now()
	e.lastFetchBytes = key.size
	e.fetchCount++
	e.fetchBytes += int64(key.size)
	e.rangeResponseBytes += int64(responseBytes)

	// Account for index-tail requests separately from payload requests.
	e.indexTailFetchCount++
	e.indexTailFetchBytes += int64(key.size)
	e.indexTailResponseBytes += int64(responseBytes)
	return e.statsChanged
}

// readResidentRange allocates an owned buffer for [start, end). It fills the
// buffer and returns true only when every byte is resident under one lock.
func (e *PackReader) readResidentRange(start, end int64) ([]byte, bool) {
	// Accept an empty resident byte interval without allocating a buffer.
	if end <= start {
		return nil, true
	}

	// Copy the requested byte interval while holding the resident span lock.
	out := make([]byte, end-start)
	var ok bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Require complete resident coverage before copying the requested bytes.
		spans, covered := e.collectSpansLocked(start, end)
		if !covered {
			return
		}

		// Copy the resident spans into the caller-owned buffer.
		ok = copySpans(out, spans, start) == len(out)
	})
	return out, ok
}

// spanIndexLocked returns the index of the first span ending after off.
func (e *PackReader) spanIndexLocked(off int64) int {
	return sort.Search(len(e.spans), func(i int) bool {
		return e.spans[i].end() > off
	})
}

// findCoveringSpanLocked returns the resident span covering off, or nil.
// Touches the span's LRU sequence if found.
func (e *PackReader) findCoveringSpanLocked(off int64) *span {
	// Locate the resident span covering the requested offset.
	i := e.spanIndexLocked(off)
	if i == len(e.spans) || e.spans[i].off > off {
		return nil
	}

	// Refresh the covering span in the resident LRU order.
	s := e.spans[i]
	e.touchSpanLocked(s)
	return s
}

// findLoadingLocked returns any in-flight load that will cover off.
func (e *PackReader) findLoadingLocked(off int64) *fetchLoad {
	for key, load := range e.loading {
		if off >= key.off && off < key.end() {
			return load
		}
	}
	return nil
}

// findGapLocked returns the uncovered byte interval around off.
//
// If off is already covered by a resident or in-flight span the gap is empty.
// Otherwise the gap is the widest [prevEnd, nextStart) that contains off,
// where prevEnd is the end of the span before off (or 0) and nextStart is the
// start of the next span (or the pack size).
func (e *PackReader) findGapLocked(off int64) (int64, int64, bool) {
	// Reject offsets before the start of the packfile.
	if off < 0 {
		return 0, 0, false
	}

	// Find the resident gap around this offset.
	i := e.spanIndexLocked(off)
	if i < len(e.spans) && e.spans[i].off <= off {
		return 0, 0, false
	}
	prevEnd := int64(0)
	if i > 0 {
		prevEnd = e.spans[i-1].end()
	}
	end := e.size
	if end <= 0 {
		end = int64(1 << 62)
	}
	if i < len(e.spans) {
		end = e.spans[i].off
	}

	// Reserve in-flight bytes too. A larger neighboring miss must not overlap
	// a smaller request whose starting offset lies inside its preferred window.
	for key := range e.loading {
		if off >= key.off && off < key.end() {
			return 0, 0, false
		}
		if key.end() <= off {
			prevEnd = max(prevEnd, key.end())
		} else if key.off > off {
			end = min(end, key.off)
		}
	}
	return prevEnd, end, off >= prevEnd && off < end
}

// insertSpanLocked inserts a span in ascending order and charges the budget.
//
// The span enters the LRU list as the newest span. The caller runs budget
// reclaim after releasing bcast.
func (e *PackReader) insertSpanLocked(s *span) {
	// Index the fetched span and account for its resident bytes and recency.
	i := sort.Search(len(e.spans), func(i int) bool {
		return e.spans[i].off >= s.off
	})
	e.spans = slices.Insert(e.spans, i, s)
	s.lru = e.lru.PushBack(s)
	e.chargeLocked(s.size)
	if len(e.spans) == 1 {
		e.budget.spansChanged(e, 1)
	}
	e.spanRef.Store(true)
}

// removeSpanLocked removes a resident span and returns its bytes to the budget.
func (e *PackReader) removeSpanLocked(s *span) {
	// Locate the resident span before removing it.
	i := sort.Search(len(e.spans), func(i int) bool {
		return e.spans[i].off >= s.off
	})
	if i == len(e.spans) || e.spans[i] != s {
		return
	}

	// Detach the resident span and return its bytes to the shared budget.
	e.spans = slices.Delete(e.spans, i, i+1)
	e.lru.Remove(s.lru)
	e.chargeLocked(-s.size)
	if len(e.spans) == 0 {
		e.budget.spansChanged(e, 0)
	}
}

// chargeLocked adds delta resident bytes to the reader and its budget.
func (e *PackReader) chargeLocked(delta int64) {
	e.residentBytes += delta
	e.budget.used.Add(delta)
}

// touchSpanLocked marks a span as the reader's most recently used and the
// reader as used for the budget's span ring.
func (e *PackReader) touchSpanLocked(s *span) {
	e.lru.MoveToBack(s.lru)
	e.spanRef.Store(true)
}

// collectSpansLocked returns the disjoint spans covering [start, end).
// Returns (nil, false) if any byte in the interval is uncovered.
func (e *PackReader) collectSpansLocked(start, end int64) ([]*span, bool) {
	// Collect resident spans until the entire requested interval is covered.
	var out []*span
	cur := start
	for _, s := range e.spans[e.spanIndexLocked(start):] {
		// Stop when a gap interrupts resident coverage.
		if cur < s.off {
			return nil, false
		}

		// Retain the next resident span and advance the covered interval.
		e.touchSpanLocked(s)
		out = append(out, s)
		cur = min(end, s.end())
		if cur >= end {
			return out, true
		}
	}
	return nil, false
}

// oldestEvictableLocked returns the least recently used span, or nil when the
// reader holds none.
//
// Every span is evictable: a fetch hands its span to its waiters and its
// writeback directly, so dropping it from the store loses no reader's bytes.
// Exempting each reader's newest span would let every pack ever read keep up to
// one fetch window beyond the shared budget.
func (e *PackReader) oldestEvictableLocked() *span {
	front := e.lru.Front()
	if front == nil {
		return nil
	}
	return front.Value.(*span)
}

// evictOldestSpan removes the reader's oldest evictable span. A closed reader
// keeps its spans until its jobs drain, so it leaves the span ring instead;
// either way the sweep never returns to a reader with nothing to give.
func (e *PackReader) evictOldestSpan() {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s := e.oldestEvictableLocked()
		if e.closed || s == nil {
			e.budget.spansChanged(e, 0)
			return
		}
		e.removeSpanLocked(s)
	})
}
