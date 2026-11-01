package store

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// residentBudget bounds the resident span bytes and parsed index entries of
// every PackReader sharing it.
//
// PackfileStore owns one budget for all of its readers, so resident memory has
// one limit however many packs are open. Readers charge span bytes as spans
// are inserted and removed, and index entries as an index is published and
// dropped. reclaim evicts the least recently used span across all readers
// until the bytes fit, and reclaimIndexes drops the least recently used
// index until the entries fit. A hot pack keeps its bytes and index while cold
// packs give theirs up; a dropped index reloads from the index cache on its
// next use.
type residentBudget struct {
	// clock orders span and index use across every reader sharing the budget.
	clock atomic.Uint64
	// limit is the byte limit. Zero or negative disables eviction.
	limit atomic.Int64
	// used is the resident span bytes charged by every reader.
	used atomic.Int64
	// indexLimit is the parsed index entry limit. Zero or negative disables
	// dropping indexes.
	indexLimit atomic.Int64
	// indexUsed is the parsed index entries charged by every reader.
	indexUsed atomic.Int64

	// mtx guards readers.
	mtx sync.Mutex
	// readers are the readers charging the budget.
	readers map[*PackReader]struct{}
}

// newResidentBudget builds a budget with the given byte limit.
func newResidentBudget(limit int64) *residentBudget {
	b := &residentBudget{readers: make(map[*PackReader]struct{})}
	b.limit.Store(limit)
	b.indexLimit.Store(defaultIndexEntryBudget)
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

// reclaimIndexes drops the least recently used parsed index until the entries
// fit. It never drops the most recently used index, so a reader that just
// loaded its index can use it even when that one index exceeds the limit.
//
// The caller must not hold any reader lock.
func (b *residentBudget) reclaimIndexes() {
	for {
		limit := b.indexLimit.Load()
		if limit <= 0 || b.indexUsed.Load() <= limit {
			return
		}
		b.mtx.Lock()
		readers := slices.Collect(maps.Keys(b.readers))
		b.mtx.Unlock()

		var victim *PackReader
		var victimSeq, newestSeq uint64
		for _, e := range readers {
			seq, ok := e.lastIndexUse()
			if !ok {
				continue
			}
			newestSeq = max(newestSeq, seq)
			if victim == nil || seq < victimSeq {
				victim, victimSeq = e, seq
			}
		}
		if victim == nil || victimSeq == newestSeq {
			return
		}
		victim.dropIndex(victimSeq)
	}
}
