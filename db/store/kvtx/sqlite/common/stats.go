package common

import (
	"sync"
	"sync/atomic"
	"time"
)

// StoreStats counts a store's waits for and holds of the writer turn and its
// passive checkpoints since it opened, so a slow commit can be split into the
// time it queued and the time another turn held the writer.
type StoreStats struct {
	// File is the main database file, empty for one in memory or not yet
	// read.
	File string
	// ForegroundWaits counts write transactions that took a foreground turn.
	ForegroundWaits uint64
	// BackgroundWaits counts write transactions that took a background turn.
	BackgroundWaits uint64
	// ForegroundWait sums foreground writers' waits for their turn.
	ForegroundWait time.Duration
	// BackgroundWait sums background writers' waits for their turn.
	BackgroundWait time.Duration
	// WaitsOver100ms counts turns that waited at least 100 milliseconds.
	WaitsOver100ms uint64
	// WaitsOver1s counts turns that waited at least one second.
	WaitsOver1s uint64
	// ForegroundHold sums foreground turns from admission through release.
	ForegroundHold time.Duration
	// BackgroundHold sums background turns from admission through release.
	// Every writer queued meanwhile waits out a hold; a foreground writer
	// goes first in the queue but cannot cut a background hold short.
	BackgroundHold time.Duration
	// ForegroundHoldsOver1s counts foreground holds of at least one second.
	ForegroundHoldsOver1s uint64
	// BackgroundHoldsOver1s counts background holds of at least one second.
	BackgroundHoldsOver1s uint64
	// Passives counts passive checkpoints.
	Passives uint64
	// Passive sums the time spent in passive checkpoints.
	Passive time.Duration
	// FlowWaits counts commits that outran the checkpointer and waited for
	// it, as walFlow describes.
	FlowWaits uint64
	// FlowWait sums those waits.
	FlowWait time.Duration
	// FlowWaitMax is the longest of them.
	FlowWaitMax time.Duration
	// LogPeakBytes is the largest log a commit left.
	LogPeakBytes int64
}

// storeStats is the atomic form of StoreStats a Store updates.
type storeStats struct {
	fgWaits, bgWaits, fgWait, bgWait, over100ms, over1s atomic.Uint64
	fgHold, bgHold, fgHoldsOver1s, bgHoldsOver1s        atomic.Uint64
	passives, passive                                   atomic.Uint64
	flowWaits, flowWait, flowWaitMax, logPeak           atomic.Uint64
}

// addWait counts a turn taken at prio after waiting d.
func (s *storeStats) addWait(prio int, d time.Duration) {
	ns := uint64(max(d, 0)) //nolint:gosec // d is not negative.
	if prio == writerBackground {
		s.bgWaits.Add(1)
		s.bgWait.Add(ns)
	} else {
		s.fgWaits.Add(1)
		s.fgWait.Add(ns)
	}
	if d >= 100*time.Millisecond {
		s.over100ms.Add(1)
	}
	if d >= time.Second {
		s.over1s.Add(1)
	}
}

// addHold counts a turn at prio that held the writer for d.
func (s *storeStats) addHold(prio int, d time.Duration) {
	// Count the hold on the counters of its priority.
	ns := uint64(max(d, 0)) //nolint:gosec // d is not negative.
	hold, over := &s.fgHold, &s.fgHoldsOver1s
	if prio == writerBackground {
		hold, over = &s.bgHold, &s.bgHoldsOver1s
	}
	hold.Add(ns)
	if d >= time.Second {
		over.Add(1)
	}
}

// addPassive counts a passive checkpoint that took d.
func (s *storeStats) addPassive(d time.Duration) {
	s.passives.Add(1)
	s.passive.Add(uint64(max(d, 0))) //nolint:gosec // d is not negative.
}

// addFlowWait counts a commit's wait d for the checkpointer.
func (s *storeStats) addFlowWait(d time.Duration) {
	ns := uint64(max(d, 0)) //nolint:gosec // d is not negative.
	s.flowWaits.Add(1)
	s.flowWait.Add(ns)
	raise(&s.flowWaitMax, ns)
}

// addLog counts the log's size after a commit.
func (s *storeStats) addLog(size int64) {
	raise(&s.logPeak, uint64(max(size, 0))) //nolint:gosec // size is not negative.
}

// raise sets v to n when n is larger.
func raise(v *atomic.Uint64, n uint64) {
	for cur := v.Load(); n > cur && !v.CompareAndSwap(cur, n); cur = v.Load() {
	}
}

// snapshot reads the counters.
func (s *storeStats) snapshot() StoreStats {
	d := func(v *atomic.Uint64) time.Duration { return time.Duration(v.Load()) } //nolint:gosec // sums of durations.
	return StoreStats{
		ForegroundWaits:       s.fgWaits.Load(),
		BackgroundWaits:       s.bgWaits.Load(),
		ForegroundWait:        d(&s.fgWait),
		BackgroundWait:        d(&s.bgWait),
		WaitsOver100ms:        s.over100ms.Load(),
		WaitsOver1s:           s.over1s.Load(),
		ForegroundHold:        d(&s.fgHold),
		BackgroundHold:        d(&s.bgHold),
		ForegroundHoldsOver1s: s.fgHoldsOver1s.Load(),
		BackgroundHoldsOver1s: s.bgHoldsOver1s.Load(),
		Passives:              s.passives.Load(),
		Passive:               d(&s.passive),
		FlowWaits:             s.flowWaits.Load(),
		FlowWait:              d(&s.flowWait),
		FlowWaitMax:           d(&s.flowWaitMax),
		LogPeakBytes:          int64(s.logPeak.Load()), //nolint:gosec // sizes fit.
	}
}

// statsSource is an open store as the registry sees it.
type statsSource interface {
	// Stats returns the store's counters.
	Stats() StoreStats
}

// openStores holds every open store, for EachStoreStats.
var openStores sync.Map

// EachStoreStats calls fn with the counters of each open store in
// this process, in no order.
func EachStoreStats(fn func(StoreStats)) {
	openStores.Range(func(k, _ any) bool {
		fn(k.(statsSource).Stats())
		return true
	})
}

// Stats returns the store's writer turn and checkpoint counters since it
// opened.
func (s *Store[T]) Stats() StoreStats {
	// Snapshot the counters and name the file they count.
	st := s.stats.snapshot()
	s.fileMtx.Lock()
	st.File = s.file
	s.fileMtx.Unlock()
	return st
}
