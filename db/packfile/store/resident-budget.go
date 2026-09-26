package store

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// residentBudget bounds the resident span bytes of every PackReader sharing it.
//
// PackfileStore owns one budget for all of its readers, so resident memory has
// one limit however many packs are open. Readers charge span bytes as spans
// are inserted and removed. reclaim evicts the least recently used span across
// all readers until the total fits. A hot pack keeps its bytes while cold packs
// give theirs up.
type residentBudget struct {
	// clock orders span use across every reader sharing the budget.
	clock atomic.Uint64
	// limit is the byte limit. Zero or negative disables eviction.
	limit atomic.Int64
	// used is the resident span bytes charged by every reader.
	used atomic.Int64

	// mtx guards readers.
	mtx sync.Mutex
	// readers are the readers charging the budget.
	readers map[*PackReader]struct{}
}

// newResidentBudget builds a budget with the given byte limit.
func newResidentBudget(limit int64) *residentBudget {
	b := &residentBudget{readers: make(map[*PackReader]struct{})}
	b.limit.Store(limit)
	return b
}

// overage returns the bytes charged beyond the limit, or zero.
func (b *residentBudget) overage() int64 {
	limit := b.limit.Load()
	if limit <= 0 {
		return 0
	}
	return max(b.used.Load()-limit, 0)
}

// attach registers a reader as an eviction source.
func (b *residentBudget) attach(e *PackReader) {
	b.mtx.Lock()
	b.readers[e] = struct{}{}
	b.mtx.Unlock()
}

// detach unregisters a reader.
func (b *residentBudget) detach(e *PackReader) {
	b.mtx.Lock()
	delete(b.readers, e)
	b.mtx.Unlock()
}

// reclaim evicts until the budget fits or nothing more can be released.
//
// The caller must not hold any reader lock: reclaim takes each reader's lock
// in turn, never two at once.
func (b *residentBudget) reclaim() {
	for b.overage() > 0 {
		b.mtx.Lock()
		readers := slices.Collect(maps.Keys(b.readers))
		b.mtx.Unlock()

		var victim *PackReader
		var victimSeq uint64
		for _, e := range readers {
			seq, ok := e.oldestEvictableSeq()
			if ok && (victim == nil || seq < victimSeq) {
				victim, victimSeq = e, seq
			}
		}
		if victim == nil {
			return
		}
		victim.evictOldestSpan()
	}
}
