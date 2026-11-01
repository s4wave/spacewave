package provider_local

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// sqlReservationsEnabled keeps the new path opt-in until production acceptance.
var sqlReservationsEnabled atomic.Bool

// SetSQLPublicationReservations enables pre-lock capacity claims for new writes.
// Configure before mounting SharedObjects and restart to change modes. Enable
// after storage acceptance; disabling retains legacy writer-lock waits.
func SetSQLPublicationReservations(enabled bool) { sqlReservationsEnabled.Store(enabled) }

// sqlReservation is capacity owned by a caller before it takes writer locks.
// The publisher's broadcast guards used and the reserved counters. Admission
// transfers its budget to pending; Release only returns an unused claim.
type sqlReservation struct {
	p    *sqlPublisher
	size int64
	used bool
	// release avoids retaking the budget mutex for a consumed claim's later owners.
	release sync.Once
}

// Bytes returns the block bytes this claim can admit.
func (r *sqlReservation) Bytes() int64 { return r.size }

// Release returns an unconsumed claim and wakes capacity waiters.
func (r *sqlReservation) Release() {
	r.release.Do(func() {
		r.p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if !r.used {
				r.used = true
				r.p.reserved--
				r.p.reservedBytes -= r.size
				broadcast()
			}
		})
	})
}

// reserve waits without a caller's writer locks until its exact payload fits.
// Like legacy admission, one oversized publication may occupy the empty queue
// alone; it never shares that exception with a second publication or claim.
func (p *sqlPublisher) reserve(ctx context.Context, size int64) (*sqlReservation, error) {
	return p.reserveCapacity(ctx, size, true)
}

// reserveCapacity optionally waits on the queue owner's capacity watch.
func (p *sqlPublisher) reserveCapacity(ctx context.Context, size int64, waiting bool) (*sqlReservation, error) {
	if size < 0 {
		return nil, errors.New("negative SQL publication capacity")
	}
	for {
		// Claim count and bytes together in the queue owner's critical section.
		var wait <-chan struct{}
		var reservation *sqlReservation
		var err error
		p.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			// A canceled caller or closed publisher owns no new capacity.
			err = ctx.Err()
			if err == nil {
				err = p.closed
			}
			if err != nil {
				return
			}

			// Count pending publications and unused claims against the same bound.
			held := p.pending + p.reserved
			if held != 0 && (held >= sqlPublishMaxPending || p.pendingBytes+p.reservedBytes+size > sqlPublishMaxPendingBytes) {
				wait = getWait()
				return
			}
			p.reserved++
			p.reservedBytes += size
			reservation = &sqlReservation{p: p, size: size}
		})

		// Cancellation before admission leaves no claim or borrowed publication.
		if err != nil {
			return nil, err
		}
		if reservation != nil {
			return reservation, nil
		}
		if !waiting {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}
