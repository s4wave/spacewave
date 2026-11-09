package common

import (
	"sync"
	"sync/atomic"
	"time"
)

// StoreStats counts a store's waits for the writer turn and its background
// checkpoints since it opened, so a slow commit can be split into the time it
// queued and the time the WAL's checkpoint held the file.
type StoreStats struct {
	// File is the main database file, empty for one in memory or not yet
	// read.
	File string
	// ForegroundWaits and BackgroundWaits count the write transactions that
	// took the writer turn at each priority, and ForegroundWait and
	// BackgroundWait sum their waits for it.
	ForegroundWaits, BackgroundWaits uint64
	ForegroundWait, BackgroundWait   time.Duration
	// WaitsOver100ms and WaitsOver1s count the turns of either priority that
	// waited at least that long.
	WaitsOver100ms, WaitsOver1s uint64
	// Passives counts the passive checkpoints and Passive sums their time.
	Passives uint64
	Passive  time.Duration
	// Truncates counts the truncating checkpoints, Truncate sums their time,
	// TruncatesOver1s counts the ones that took at least a second, and
	// LastTruncate is the time the newest took. A truncate holds SQLite's
	// write lock, so every writer waits for it.
	Truncates, TruncatesOver1s uint64
	Truncate, LastTruncate     time.Duration
}

// storeStats is the atomic form of StoreStats a Store updates.
type storeStats struct {
	fgWaits, bgWaits, fgWait, bgWait, over100ms, over1s atomic.Uint64
	passives, passive                                   atomic.Uint64
	truncates, truncatesOver1s, truncate, lastTruncate  atomic.Uint64
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

// addPassive counts a passive checkpoint that took d.
func (s *storeStats) addPassive(d time.Duration) {
	s.passives.Add(1)
	s.passive.Add(uint64(max(d, 0))) //nolint:gosec // d is not negative.
}

// addTruncate counts a truncating checkpoint that took d.
func (s *storeStats) addTruncate(d time.Duration) {
	ns := uint64(max(d, 0)) //nolint:gosec // d is not negative.
	s.truncates.Add(1)
	s.truncate.Add(ns)
	s.lastTruncate.Store(ns)
	if d >= time.Second {
		s.truncatesOver1s.Add(1)
	}
}

// snapshot reads the counters.
func (s *storeStats) snapshot() StoreStats {
	d := func(v *atomic.Uint64) time.Duration { return time.Duration(v.Load()) } //nolint:gosec // sums of durations.
	return StoreStats{
		ForegroundWaits: s.fgWaits.Load(),
		BackgroundWaits: s.bgWaits.Load(),
		ForegroundWait:  d(&s.fgWait),
		BackgroundWait:  d(&s.bgWait),
		WaitsOver100ms:  s.over100ms.Load(),
		WaitsOver1s:     s.over1s.Load(),
		Passives:        s.passives.Load(),
		Passive:         d(&s.passive),
		Truncates:       s.truncates.Load(),
		TruncatesOver1s: s.truncatesOver1s.Load(),
		Truncate:        d(&s.truncate),
		LastTruncate:    d(&s.lastTruncate),
	}
}

// statsSource is an open store as the registry sees it.
type statsSource interface {
	// Stats returns the store's counters.
	Stats() StoreStats
}

// openStores holds every open writable store, for EachStoreStats.
var openStores sync.Map

// EachStoreStats calls fn with the counters of each open writable store in
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
	st := s.stats.snapshot()
	s.fileMtx.Lock()
	st.File = s.file
	s.fileMtx.Unlock()
	return st
}
