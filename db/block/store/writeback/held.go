package block_store_writeback

import (
	"bytes"
	"maps"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// heldRecheckMin and heldRecheckMax bound the wait before the upload reads the
// held entries again. The wait doubles between them while blocks stay held,
// since a read fails the same way until something repairs the block.
const (
	heldRecheckMin = 30 * time.Second
	heldRecheckMax = 10 * time.Minute
)

// heldEntry is a queue entry the upload read corrupt: its local bytes fail
// their hash. It keeps its marker and local value, so the queue head, the
// eviction rule and UploadedBefore all still count it as not uploaded.
type heldEntry struct {
	// value is the queue entry's value when the upload read it. A write of
	// the block since then changes the value, which makes the entry eligible.
	value []byte
	// retry is set when the recheck timer put the entry back for one read.
	retry bool
}

// heldView is the held set one read of the queue skips.
type heldView struct {
	entries map[string]heldEntry
	// gen is the recheck count the view was taken at.
	gen uint64
}

// eligible reports whether the queue entry at key with value may upload: it
// is not held, the recheck put it back, or a write changed it since it was
// held.
func (v heldView) eligible(key, value []byte) bool {
	e, ok := v.entries[string(key)]
	return !ok || e.retry || !bytes.Equal(e.value, value)
}

// heldSnapshot returns the held set for one read of the queue.
func (s *Store) heldSnapshot() heldView {
	var v heldView
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		v = heldView{entries: s.held, gen: s.heldGen}
	})
	return v
}

// recordHeld records what a batch found: an entry read corrupt is held at the
// value it was read at, and any other entry of the batch leaves the held set.
// It derives the status's held state from the set and arms the recheck.
func (s *Store) recordHeld(batch []pendingBlock) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Build the next set, replacing the map only when the batch changes it.
		var next map[string]heldEntry
		for i := range batch {
			key := string(batch[i].queueKey)
			_, was := s.held[key]
			if !batch[i].corrupt && !was {
				continue
			}
			if next == nil {
				next = maps.Clone(s.held)
				if next == nil {
					next = make(map[string]heldEntry)
				}
			}
			if batch[i].corrupt {
				next[key] = heldEntry{value: batch[i].value}
			} else {
				delete(next, key)
			}
		}
		if next == nil {
			return
		}

		// Show the new state and schedule the next read of the held blocks.
		s.setHeldLocked(next)
		s.armHeldLocked()
		broadcast()
	})
}

// setHeldLocked replaces the held set and derives the status's held state
// from it: the count is the set's size, and the time is when the set last
// became non-empty. The caller holds bcast.
func (s *Store) setHeldLocked(held map[string]heldEntry) {
	s.held = held
	switch {
	case len(held) == 0:
		s.status.Held, s.heldDelay = nil, 0
	case s.status.Held == nil:
		s.status.Held = &block.HeldError{Count: len(held), Since: time.Now()}
	default:
		s.status.Held = &block.HeldError{Count: len(held), Since: s.status.Held.Since}
	}
}

// armHeldLocked starts the recheck timer while entries are held, none waits
// for its recheck read and the timer is not running, so a block that stays
// corrupt is read at most once per delay. The caller holds bcast.
func (s *Store) armHeldLocked() {
	// Schedule another read only after the held entries have become idle.
	if s.heldArmed || len(s.held) == 0 {
		return
	}
	for _, e := range s.held {
		if e.retry {
			return
		}
	}
	s.heldDelay = min(max(2*s.heldDelay, heldRecheckMin), heldRecheckMax)
	s.heldArmed = true
	time.AfterFunc(s.heldDelay, s.recheckHeld)
}

// recheckHeld puts every held entry back for one more read, so a block
// repaired by a refill since it was held uploads and leaves the set.
func (s *Store) recheckHeld() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Make every held entry eligible for another upload read and wake it.
		s.heldArmed = false
		next := make(map[string]heldEntry, len(s.held))
		for k, e := range s.held {
			e.retry = true
			next[k] = e
		}
		s.held = next
		s.heldGen++
		broadcast()
	})
}
