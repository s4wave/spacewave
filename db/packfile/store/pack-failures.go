package store

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
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
// reader or index failed to load is tried again once per backoff interval,
// growing from packRetryMin to packRetryMax, so a lost pack costs one fetch
// per interval rather than one per lookup that reaches it. A pack that failed
// on a block read keeps no backoff, since its other blocks may still read.
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

// backingOff returns the pack's last failure while it waits out its backoff,
// and nil when the pack may be tried.
func (f *packFailures) backingOff(id string, now time.Time) error {
	// Skip the lock while no pack fails.
	if f.count.Load() == 0 {
		return nil
	}

	// Answer from the pack's record.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	p := f.packs[id]
	if p == nil || p.retryAt.IsZero() || !now.Before(p.retryAt) {
		return nil
	}
	return errors.Wrapf(p.err, "pack %s waiting until %s to retry", id, p.retryAt.Format(time.RFC3339))
}

// failed records a failure of the pack, logging it when the pack was not
// failing already. loadFailed starts or grows the pack's backoff.
func (f *packFailures) failed(id string, err error, loadFailed bool, now time.Time) {
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
	p.retryAt = now.Add(p.backoff)
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
