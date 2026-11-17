package store

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// Backoff of a pack whose reader or index failed to load.
const (
	// packRetryMin is the first wait before a failed pack is tried again.
	packRetryMin = time.Second
	// packRetryMax caps the wait, so a pack that recovers is read again
	// within this long.
	packRetryMax = time.Minute
)

// packFailures tracks the packs a probe could not read. Each pack is logged
// when it first fails and when it reads again, never per lookup. A pack whose
// reader or index failed to load backs off, from packRetryMin growing to
// packRetryMax: while it does, a probe puts it after every other candidate
// and tries it only when none of them holds the block, so a lost pack whose
// blocks have other holders costs one fetch per interval rather than one per
// lookup, and a pack that is the last possible holder is always tried. The
// backoff orders and thins retries; it never turns a possible read into an
// error. A pack that failed on a block read keeps no backoff, since its other
// blocks may still read. The list holds only packs in the manifest: a pack
// leaves it when it reads again or when a manifest update drops it.
type packFailures struct {
	// count is the number of packs listed, so a probe skips the lock while
	// none fail.
	count atomic.Int32
	// mtx guards le and packs.
	mtx sync.Mutex
	// le logs the failures and recoveries, or is nil.
	le *logrus.Entry
	// packs holds each failing pack by id.
	packs map[string]*packFailure
	// now reads the clock, or is nil for time.Now.
	now func() time.Time
}

// packFailure is one failing pack.
type packFailure struct {
	// err is the latest failure.
	err error
	// backoff is the current wait, zero for a failed block read.
	backoff time.Duration
	// retryAt is when the pack is next tried; zero tries it at once.
	retryAt time.Time
}

// SetLogger sets the logger that reports packs a lookup passed over, once
// when each starts failing and once when it reads again.
func (s *PackfileStore) SetLogger(le *logrus.Entry) {
	s.failures.mtx.Lock()
	s.failures.le = le
	s.failures.mtx.Unlock()
}

// clock returns the current time.
func (f *packFailures) clock() time.Time {
	if f.now != nil {
		return f.now()
	}
	return time.Now()
}

// backingOff reports whether the pack waits out a failed load's backoff, so a
// probe tries it only after every other candidate.
func (f *packFailures) backingOff(id string) bool {
	// Skip the lock while no pack fails.
	if f.count.Load() == 0 {
		return false
	}

	// Answer from the pack's record.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	p := f.packs[id]
	return p != nil && !p.retryAt.IsZero() && f.clock().Before(p.retryAt)
}

// failed records a failure of the pack, logging it when the pack was not
// failing already. loadFailed starts or grows the pack's backoff.
func (f *packFailures) failed(id string, err error, loadFailed bool) {
	// Find or add the pack's record, logging a new one.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	p := f.packs[id]
	if p == nil {
		if f.packs == nil {
			f.packs = make(map[string]*packFailure)
		}
		p = &packFailure{}
		f.packs[id] = p
		f.count.Add(1)
		if f.le != nil {
			f.le.WithError(err).WithField("pack", id).Warn("pack unreadable, passing over it")
		}
	}

	// Record the failure, and back off a failed load.
	p.err = err
	if !loadFailed {
		p.backoff, p.retryAt = 0, time.Time{}
		return
	}
	p.backoff = min(max(p.backoff*2, packRetryMin), packRetryMax)
	p.retryAt = f.clock().Add(p.backoff)
}

// recovered forgets the pack's failure after it read, logging the recovery.
func (f *packFailures) recovered(id string) {
	// Skip the lock while no pack fails.
	if f.count.Load() == 0 {
		return
	}

	// Drop the record, logging that the pack reads again.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if _, ok := f.packs[id]; !ok {
		return
	}
	delete(f.packs, id)
	f.count.Add(-1)
	if f.le != nil {
		f.le.WithField("pack", id).Info("pack readable again")
	}
}

// retain drops the packs keep rejects, such as those a manifest update
// removed, so the list holds only packs in the manifest.
func (f *packFailures) retain(keep func(id string) bool) {
	// Skip the lock while no pack fails.
	if f.count.Load() == 0 {
		return
	}

	// Drop each rejected pack's record.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	for id := range f.packs {
		if !keep(id) {
			delete(f.packs, id)
			f.count.Add(-1)
		}
	}
}
