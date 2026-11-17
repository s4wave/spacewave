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
	// within this long. A record of a failed block read expires after it.
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
// blocks may still read, and its record expires packRetryMax after the
// failure. The list holds only packs in the manifest: a pack leaves it when it
// reads again, when its record expires, or when a manifest update drops it.
//
// Lookups read two immutable snapshots, republished on each change, so a
// probe takes no lock: listed holds the id of every recorded pack, and
// backoffs the retry time of each pack backing off.
type packFailures struct {
	// listed is the set of recorded pack ids, or nil when none.
	listed atomic.Pointer[map[string]struct{}]
	// backoffs is the retry time of each pack backing off, or nil when none.
	backoffs atomic.Pointer[map[string]time.Time]
	// mtx guards le and packs, and serializes publishing the snapshots.
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
	// expireAt is when the record of a failed block read lapses, or zero.
	expireAt time.Time
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
// probe tries it only after every other candidate. It takes no lock.
func (f *packFailures) backingOff(id string) bool {
	m := f.backoffs.Load()
	if m == nil {
		return false
	}
	retryAt, ok := (*m)[id]
	return ok && f.clock().Before(retryAt)
}

// failed records a failure of the pack, logging it when the pack was not
// failing already. loadFailed starts or grows the pack's backoff; otherwise
// the record expires packRetryMax from now.
func (f *packFailures) failed(id string, err error, loadFailed bool) {
	// Find or add the pack's record, logging a new one.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	now := f.clock()
	f.expireLocked(now)
	p := f.packs[id]
	if p == nil {
		if f.packs == nil {
			f.packs = make(map[string]*packFailure)
		}
		p = &packFailure{}
		f.packs[id] = p
		if f.le != nil {
			f.le.WithError(err).WithField("pack", id).Warn("pack unreadable, passing over it")
		}
	}

	// Record the failure: back off a failed load, and let any other lapse.
	p.err = err
	if loadFailed {
		p.backoff = min(max(p.backoff*2, packRetryMin), packRetryMax)
		p.retryAt, p.expireAt = now.Add(p.backoff), time.Time{}
	} else {
		p.backoff, p.retryAt, p.expireAt = 0, time.Time{}, now.Add(packRetryMax)
	}
	f.publishLocked()
}

// recovered forgets the pack's failure after it read, logging the recovery.
// A pack with no record costs a snapshot load and a map lookup.
func (f *packFailures) recovered(id string) {
	// Skip the lock unless the pack is listed.
	if m := f.listed.Load(); m == nil {
		return
	} else if _, ok := (*m)[id]; !ok {
		return
	}

	// Drop the record, logging that the pack reads again.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if _, ok := f.packs[id]; !ok {
		return
	}
	delete(f.packs, id)
	f.publishLocked()
	if f.le != nil {
		f.le.WithField("pack", id).Info("pack readable again")
	}
}

// retain drops the packs keep rejects, such as those a manifest update
// removed, and the lapsed records, so the list holds only packs in the
// manifest.
func (f *packFailures) retain(keep func(id string) bool) {
	// Skip the lock while no pack is listed.
	if f.listed.Load() == nil {
		return
	}

	// Drop each rejected or lapsed pack's record.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.expireLocked(f.clock())
	for id := range f.packs {
		if !keep(id) {
			delete(f.packs, id)
		}
	}
	f.publishLocked()
}

// expireLocked drops the records of failed block reads that have lapsed.
func (f *packFailures) expireLocked(now time.Time) {
	for id, p := range f.packs {
		if !p.expireAt.IsZero() && !now.Before(p.expireAt) {
			delete(f.packs, id)
		}
	}
}

// publishLocked republishes the snapshots lookups read from the records.
func (f *packFailures) publishLocked() {
	// Publish nil snapshots when nothing is listed or backing off.
	if len(f.packs) == 0 {
		f.listed.Store(nil)
		f.backoffs.Store(nil)
		return
	}

	// Copy the listed ids and the retry times of the packs backing off.
	listed := make(map[string]struct{}, len(f.packs))
	var backoffs map[string]time.Time
	for id, p := range f.packs {
		listed[id] = struct{}{}
		if !p.retryAt.IsZero() {
			if backoffs == nil {
				backoffs = make(map[string]time.Time)
			}
			backoffs[id] = p.retryAt
		}
	}
	f.listed.Store(&listed)
	if backoffs == nil {
		f.backoffs.Store(nil)
	} else {
		f.backoffs.Store(&backoffs)
	}
}
