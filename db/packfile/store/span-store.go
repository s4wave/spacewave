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

// ensureResident fetches every uncovered byte of [start, end).
//
// exact limits each fetch to the uncovered gap, as index-tail reads need.
// Otherwise the planner may widen fetches for read-ahead.
func (e *PackReader) ensureResident(ctx context.Context, start, end int64, exact bool) error {
	for cur := start; cur < end; {
		var resident *span
		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			resident = e.findCoveringSpanLocked(cur)
		})
		if resident != nil {
			cur = min(end, resident.end())
			continue
		}
		if err := e.fetchRange(ctx, cur, end, exact); err != nil {
			return err
		}
	}
	return nil
}

// fetchRange drives a transport fetch to cover off, bounded by readEnd.
//
// It returns once the requested offset is resident or an error occurs. Other
// concurrent callers for overlapping offsets fold onto the same in-flight
// fetch via the loading map, guaranteeing one transport call per uncovered
// span. Transport work belongs to the PackReader, so canceling the caller
// that starts a fetch does not cancel another caller waiting for it. exact
// selects the gap-clipped index planner instead of the adaptive planner.
func (e *PackReader) fetchRange(ctx context.Context, off, readEnd int64, exact bool) error {
	ctx, task := trace.NewTask(ctx, "provider/spacewave/packfile/range-fetch")
	defer task.End()
	trace.Log(ctx, "pack-id", e.packID)
	trace.Logf(ctx, "target-offset", "%d", off)
	trace.Logf(ctx, "target-end", "%d", readEnd)
	trace.Logf(ctx, "exact", "%t", exact)

	if err := ctx.Err(); err != nil {
		return err
	}

	var resident, closed, started bool
	var key fetchKey
	var load *fetchLoad
	var notifyStart func()
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			closed = true
			return
		}
		if e.findCoveringSpanLocked(off) != nil {
			resident = true
			return
		}
		load = e.findLoadingLocked(off)
		if load != nil {
			return
		}
		if exact {
			key = e.planExactFetchLocked(off, readEnd)
		} else {
			key = e.planFetchLocked(off, readEnd, block.ReadAhead(ctx))
		}
		if key.size == 0 {
			return
		}
		if e.loading == nil {
			e.loading = make(map[fetchKey]*fetchLoad)
		}
		load = &fetchLoad{done: make(chan struct{})}
		e.loading[key] = load
		e.workCount++
		started = true
		notifyStart = e.statsChanged
	})

	if notifyStart != nil {
		notifyStart()
	}
	if closed {
		return context.Canceled
	}
	if resident {
		trace.Log(ctx, "result", "resident")
		return nil
	}
	if load == nil {
		trace.Log(ctx, "result", "empty-plan")
		return io.EOF
	}
	if started {
		trace.Log(ctx, "role", "leader")
		trace.Logf(ctx, "range-offset", "%d", key.off)
		trace.Logf(ctx, "range-size", "%d", key.size)
		e.startFetch(key, load, exact)
	} else {
		trace.Log(ctx, "role", "waiter")
	}

	select {
	case <-ctx.Done():
		trace.Log(ctx, "result", "wait-canceled")
		return ctx.Err()
	case <-e.ctx.Done():
		trace.Log(ctx, "result", "owner-canceled")
		return context.Canceled
	case <-load.done:
		if load.err != nil {
			trace.Log(ctx, "result", "wait-error")
			return load.err
		}
		if load.sp == nil {
			trace.Log(ctx, "result", "wait-empty-response")
			return io.EOF
		}
		trace.Log(ctx, "result", "waited")
		return nil
	}
}

// startFetch runs one transport request under the PackReader lifetime.
func (e *PackReader) startFetch(key fetchKey, load *fetchLoad, indexTail bool) {
	startOwnerWork(func() {
		defer e.finishOwnerWork()

		data, err := e.transport.Fetch(e.ctx, key.off, key.size)
		var sp *span
		if len(data) != 0 {
			sp = newSpan(key.off, data)
		}

		var notifyDone func()
		var writeback func()
		var budget *residentBudget
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			budget = e.budget
			if e.closed {
				sp = nil
				err = context.Canceled
			}
			if err == nil && sp != nil {
				e.insertSpanLocked(sp)
				if indexTail {
					notifyDone = e.recordIndexTailFetchLocked(key, len(data))
				} else {
					notifyDone = e.recordFetchLocked(key, len(data))
				}
				writeback = e.prepareWritebackLocked(sp.off, sp.end())
			}
			if notifyDone == nil {
				notifyDone = e.statsChanged
			}
			broadcast()
		})
		budget.reclaim()
		e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			load.sp = sp
			load.err = err
			if e.loading[key] == load {
				delete(e.loading, key)
			}
			close(load.done)
			broadcast()
		})
		if notifyDone != nil {
			notifyDone()
		}
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
	gapStart, gapEnd, ok := e.findGapLocked(off)
	if !ok || gapEnd <= gapStart {
		return fetchKey{}
	}
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
	if !e.lastTargetSet {
		return false
	}
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
	if !e.sparseReads {
		return
	}
	e.lastTargetOff = off
	e.lastTargetEnd = end
	e.lastTargetSet = true
}

// planExactFetchLocked clips index reads to the currently uncovered gap.
func (e *PackReader) planExactFetchLocked(off, readEnd int64) fetchKey {
	gapStart, gapEnd, ok := e.findGapLocked(off)
	if !ok || gapEnd <= gapStart {
		return fetchKey{}
	}
	start := max(off, gapStart)
	end := min(readEnd, gapEnd)
	if end <= start {
		end = start + 1
	}
	if end > gapEnd {
		end = gapEnd
	}
	if end <= start {
		return fetchKey{}
	}
	return fetchKey{off: start, size: int(end - start)}
}

// recordFetchLocked notes a completed fetch for adaptive sizing.
func (e *PackReader) recordFetchLocked(key fetchKey, responseBytes int) func() {
	e.lastFetchAt = time.Now()
	e.lastFetchBytes = key.size
	e.fetchCount++
	e.fetchBytes += int64(key.size)
	e.rangeResponseBytes += int64(responseBytes)
	return e.statsChanged
}

// recordIndexTailFetchLocked accounts for index I/O separately from payloads.
func (e *PackReader) recordIndexTailFetchLocked(key fetchKey, responseBytes int) func() {
	e.lastFetchAt = time.Now()
	e.lastFetchBytes = key.size
	e.fetchCount++
	e.fetchBytes += int64(key.size)
	e.rangeResponseBytes += int64(responseBytes)
	e.indexTailFetchCount++
	e.indexTailFetchBytes += int64(key.size)
	e.indexTailResponseBytes += int64(responseBytes)
	return e.statsChanged
}

// readResidentRange returns an owned copy only when every byte is resident.
func (e *PackReader) readResidentRange(start, end int64) ([]byte, bool) {
	if end <= start {
		return nil, true
	}
	out := make([]byte, end-start)
	var ok bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		spans, covered := e.collectSpansLocked(start, end)
		if !covered {
			return
		}
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
	i := e.spanIndexLocked(off)
	if i == len(e.spans) || e.spans[i].off > off {
		return nil
	}
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
	i := sort.Search(len(e.spans), func(i int) bool {
		return e.spans[i].off >= s.off
	})
	e.spans = slices.Insert(e.spans, i, s)
	s.lru = e.lru.PushBack(s)
	e.newest = s
	e.chargeLocked(s.size)
	e.touchSpanLocked(s)
}

// removeSpanLocked removes a resident span and returns its bytes to the budget.
func (e *PackReader) removeSpanLocked(s *span) {
	i := sort.Search(len(e.spans), func(i int) bool {
		return e.spans[i].off >= s.off
	})
	if i == len(e.spans) || e.spans[i] != s {
		return
	}
	e.spans = slices.Delete(e.spans, i, i+1)
	e.lru.Remove(s.lru)
	if e.newest == s {
		e.newest = nil
	}
	e.chargeLocked(-s.size)
}

// chargeLocked adds delta resident bytes to the reader and its budget.
func (e *PackReader) chargeLocked(delta int64) {
	e.residentBytes += delta
	e.budget.used.Add(delta)
}

// nextUseSeqLocked returns the next budget-wide LRU sequence.
func (e *PackReader) nextUseSeqLocked() uint64 {
	return e.budget.clock.Add(1)
}

// touchSpanLocked marks a span as the most recently used.
func (e *PackReader) touchSpanLocked(s *span) {
	s.lastUseSeq = e.nextUseSeqLocked()
	e.lru.MoveToBack(s.lru)
}

// collectSpansLocked returns the disjoint spans covering [start, end).
// Returns (nil, false) if any byte in the interval is uncovered.
func (e *PackReader) collectSpansLocked(start, end int64) ([]*span, bool) {
	var out []*span
	cur := start
	for _, s := range e.spans[e.spanIndexLocked(start):] {
		if cur < s.off {
			return nil, false
		}
		e.touchSpanLocked(s)
		out = append(out, s)
		cur = min(end, s.end())
		if cur >= end {
			return out, true
		}
	}
	return nil, false
}

// oldestEvictableLocked returns the least recently used span.
//
// The span inserted last is never returned: a caller that fetched it may not
// have read it yet.
func (e *PackReader) oldestEvictableLocked() *span {
	front := e.lru.Front()
	if front == nil {
		return nil
	}
	s := front.Value.(*span)
	if s == e.newest {
		return nil
	}
	return s
}

// oldestEvictableSeq returns the LRU sequence of the reader's oldest
// evictable span.
func (e *PackReader) oldestEvictableSeq() (uint64, bool) {
	var seq uint64
	var ok bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		if s := e.oldestEvictableLocked(); s != nil {
			seq, ok = s.lastUseSeq, true
		}
	})
	return seq, ok
}

// evictOldestSpan removes the reader's oldest evictable span.
func (e *PackReader) evictOldestSpan() {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.closed {
			return
		}
		if s := e.oldestEvictableLocked(); s != nil {
			e.removeSpanLocked(s)
		}
	})
}
