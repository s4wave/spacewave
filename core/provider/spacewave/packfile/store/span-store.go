package store

import (
	"context"
	"io"
	"time"
)

// fetchMiss drives a transport fetch to cover off, bounded by readEnd.
//
// It returns once the requested offset is resident or an error occurs. Other
// concurrent callers for overlapping offsets fold onto the same in-flight
// fetch via the loading map, guaranteeing one transport call per uncovered
// span.
func (e *PackReader) fetchMiss(ctx context.Context, off, readEnd int64) error {
	for {
		var leader, resident bool
		var key fetchKey
		var load *fetchLoad
		var notifyStart func()

		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if e.findCoveringSpanLocked(off) != nil {
				resident = true
				return
			}
			load = e.findLoadingLocked(off)
			if load != nil {
				return
			}
			key = e.planFetchLocked(off, readEnd)
			if key.size == 0 {
				return
			}
			if e.loading == nil {
				e.loading = make(map[fetchKey]*fetchLoad)
			}
			if existing, ok := e.loading[key]; ok {
				load = existing
				return
			}
			load = &fetchLoad{done: make(chan struct{})}
			e.loading[key] = load
			leader = true
			notifyStart = e.statsChanged
		})

		if notifyStart != nil {
			notifyStart()
		}
		if resident {
			return nil
		}
		if load == nil && key.size == 0 {
			return io.EOF
		}

		if leader {
			data, err := e.transport.Fetch(ctx, key.off, key.size)
			var sp *span
			if len(data) != 0 {
				sp = newSpan(key.off, e.pageSize, data)
			}
			var notifyDone func()
			e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if e.loading != nil {
					if lod := e.loading[key]; lod != nil {
						lod.sp = sp
						lod.err = err
						delete(e.loading, key)
						close(lod.done)
					}
				}
				if err == nil && sp != nil {
					e.insertSpanLocked(sp)
					notifyDone = e.recordFetchLocked(key, len(data))
					e.promoteBlocksInSpanLocked(sp)
				}
				if notifyDone == nil {
					notifyDone = e.statsChanged
				}
				broadcast()
			})
			if notifyDone != nil {
				notifyDone()
			}
			if err != nil {
				return err
			}
			if sp == nil {
				return io.EOF
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-load.done:
			if load.err != nil {
				return load.err
			}
			if load.sp == nil {
				return io.EOF
			}
			return nil
		}
	}
}

func (e *PackReader) ensureExactRangeResident(ctx context.Context, start, end int64) error {
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
		if err := e.fetchExact(ctx, cur, end); err != nil {
			return err
		}
	}
	return nil
}

func (e *PackReader) fetchExact(ctx context.Context, off, readEnd int64) error {
	for {
		var leader, resident bool
		var key fetchKey
		var load *fetchLoad
		var notifyStart func()

		e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if e.findCoveringSpanLocked(off) != nil {
				resident = true
				return
			}
			load = e.findLoadingLocked(off)
			if load != nil {
				return
			}
			key = e.planExactFetchLocked(off, readEnd)
			if key.size == 0 {
				return
			}
			if e.loading == nil {
				e.loading = make(map[fetchKey]*fetchLoad)
			}
			if existing, ok := e.loading[key]; ok {
				load = existing
				return
			}
			load = &fetchLoad{done: make(chan struct{})}
			e.loading[key] = load
			leader = true
			notifyStart = e.statsChanged
		})

		if notifyStart != nil {
			notifyStart()
		}
		if resident {
			return nil
		}
		if load == nil && key.size == 0 {
			return io.EOF
		}

		if leader {
			data, err := e.transport.Fetch(ctx, key.off, key.size)
			var sp *span
			if len(data) != 0 {
				sp = newSpan(key.off, e.pageSize, data)
			}
			var notifyDone func()
			e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if e.loading != nil {
					if lod := e.loading[key]; lod != nil {
						lod.sp = sp
						lod.err = err
						delete(e.loading, key)
						close(lod.done)
					}
				}
				if err == nil && sp != nil {
					e.insertSpanLocked(sp)
					notifyDone = e.recordIndexTailFetchLocked(key, len(data))
					e.promoteBlocksInSpanLocked(sp)
				}
				if notifyDone == nil {
					notifyDone = e.statsChanged
				}
				broadcast()
			})
			if notifyDone != nil {
				notifyDone()
			}
			if err != nil {
				return err
			}
			if sp == nil {
				return io.EOF
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-load.done:
			if load.err != nil {
				return load.err
			}
			if load.sp == nil {
				return io.EOF
			}
			return nil
		}
	}
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
func (e *PackReader) planFetchLocked(off, readEnd int64) fetchKey {
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

func (e *PackReader) recordSparseTargetLocked(off, end int64) {
	if !e.sparseReads {
		return
	}
	e.lastTargetOff = off
	e.lastTargetEnd = end
	e.lastTargetSet = true
}

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

// findCoveringSpanLocked returns the resident span covering off, or nil.
// Touches the span's LRU sequence if found.
func (e *PackReader) findCoveringSpanLocked(off int64) *span {
	for _, s := range e.spans {
		if off < s.off {
			return nil
		}
		if off < s.end() {
			e.touchSpanLocked(s)
			return s
		}
	}
	return nil
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
// If off is already covered by a resident span the gap is empty. Otherwise
// the gap is the widest [prevEnd, nextStart) that contains off, where
// prevEnd is the end of the span before off (or 0) and nextStart is the
// start of the next span (or the pack size).
func (e *PackReader) findGapLocked(off int64) (int64, int64, bool) {
	if off < 0 {
		return 0, 0, false
	}
	prevEnd := int64(0)
	for _, s := range e.spans {
		if off < s.off {
			return prevEnd, s.off, off >= prevEnd
		}
		if off < s.end() {
			return 0, 0, false
		}
		prevEnd = s.end()
	}
	end := e.size
	if end <= 0 {
		end = int64(1 << 62)
	}
	return prevEnd, end, off >= prevEnd && off < end
}

// insertSpanLocked inserts a span in ascending order and applies eviction.
func (e *PackReader) insertSpanLocked(s *span) {
	idx := 0
	for idx < len(e.spans) && e.spans[idx].off < s.off {
		idx++
	}
	e.spans = append(e.spans, nil)
	copy(e.spans[idx+1:], e.spans[idx:])
	e.spans[idx] = s
	e.residentBytes += s.size
	e.touchSpanLocked(s)
	e.evictLocked()
}

// touchSpanLocked advances the LRU sequence.
func (e *PackReader) touchSpanLocked(s *span) {
	e.useSeq++
	s.lastUseSeq = e.useSeq
}

// collectSpansLocked returns the disjoint spans covering [start, end).
// Returns (nil, false) if any byte in the interval is uncovered.
func (e *PackReader) collectSpansLocked(start, end int64) ([]*span, bool) {
	var out []*span
	cur := start
	for _, s := range e.spans {
		if cur < s.off {
			return nil, false
		}
		if cur >= s.end() {
			continue
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

// retainSpansLocked pins each span, incrementing pin counts.
// Returns the bytes that transitioned from unpinned to pinned.
func (e *PackReader) retainSpansLocked(spans []*span) int64 {
	var retained int64
	for _, s := range spans {
		if s == nil {
			continue
		}
		if s.pins == 0 {
			retained += s.size
		}
		s.pins++
		e.touchSpanLocked(s)
	}
	return retained
}

// releaseSpansLocked decrements pin counts on each span.
// Returns the bytes that transitioned from pinned to unpinned.
func (e *PackReader) releaseSpansLocked(spans []*span) int64 {
	var released int64
	for _, s := range spans {
		if s == nil || s.pins == 0 {
			continue
		}
		s.pins--
		if s.pins == 0 {
			released += s.size
		}
	}
	return released
}

// removeUnpinnedSpansLocked removes any matching resident spans that are no
// longer pinned.
func (e *PackReader) removeUnpinnedSpansLocked(spans []*span) {
	for _, victim := range spans {
		if victim == nil || victim.pins != 0 {
			continue
		}
		for i, resident := range e.spans {
			if resident != victim {
				continue
			}
			e.spans = append(e.spans[:i], e.spans[i+1:]...)
			e.residentBytes -= victim.size
			if e.residentBytes < 0 {
				e.residentBytes = 0
			}
			break
		}
	}
}

// evictLocked evicts least-recently-used unpinned spans over the byte budget.
//
// Pinned spans are never evicted (a block record holds them). The newest
// span (lastUseSeq == useSeq) is never evicted in the same call that
// inserted it to prevent immediate re-eviction of the span that just landed.
func (e *PackReader) evictLocked() {
	if e.maxBytes <= 0 {
		return
	}
	for e.residentBytes > e.maxBytes {
		var victimSpan *span
		var victimSpanIdx int
		for i, s := range e.spans {
			if s.pins != 0 {
				continue
			}
			if s.lastUseSeq == e.useSeq {
				continue
			}
			if victimSpan == nil || s.lastUseSeq < victimSpan.lastUseSeq {
				victimSpan = s
				victimSpanIdx = i
			}
		}
		if victimSpan != nil {
			e.spans = append(e.spans[:victimSpanIdx], e.spans[victimSpanIdx+1:]...)
			e.residentBytes -= victimSpan.size
			if e.residentBytes < 0 {
				e.residentBytes = 0
			}
			continue
		}

		victimRecord := e.pickEvictionRecordLocked()
		if victimRecord == nil {
			return
		}
		e.removeBlockLocked(victimRecord)
	}
}
