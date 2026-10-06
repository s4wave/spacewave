package s4db

import (
	"context"
	"hash/fnv"
	"sync"
)

// Lease is a named lock held across transactions. One holder at a time, in
// any process, holds a name; the system releases the lock when the holding
// process dies.
type Lease struct {
	// db is the database the lease locks.
	db *DB
	// off is the lease's lock byte.
	off int64

	// once releases the lease once.
	once sync.Once
	// err is the result of the release.
	err error
}

// leaseOff returns the lock byte of the lease name. Two names may share a
// byte; they then exclude each other, which costs contention but never
// correctness.
func leaseOff(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return lockLease + int64(h.Sum64()%leaseLocks) // #nosec G115 -- the remainder is below leaseLocks.
}

// TryLease takes the lease name when no other holder has it, reporting false
// when one does.
func (db *DB) TryLease(name string) (*Lease, bool, error) {
	// Claim the byte in this handle.
	off := leaseOff(name)
	var claimed bool
	var err error
	db.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		claimed, err = db.claimLeaseLocked(off)
	})
	if err != nil || !claimed {
		return nil, false, err
	}

	// Lock it against other processes.
	ok, err := db.s.lock(off, false)
	if err != nil || !ok {
		db.dropLease(off)
		return nil, false, err
	}
	return &Lease{db: db, off: off}, true, nil
}

// WaitLease waits until it holds the lease name. When ctx ends while another
// process holds the name, the abandoned wait keeps the name claimed in this
// handle until the system grants the lock, then releases it; Close waits for
// it.
func (db *DB) WaitLease(ctx context.Context, name string) (*Lease, error) {
	// Wait for this handle's holder to release the byte, then claim it.
	off := leaseOff(name)
	err := db.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return db.claimLeaseLocked(off)
	})
	if err != nil {
		return nil, err
	}

	// Lock the byte in the background so the wait follows ctx.
	db.lockWaits.Add(1)
	locked := make(chan error, 1)
	go func() {
		_, err := db.s.lock(off, true)
		locked <- err
	}()
	select {
	case err := <-locked:
		db.lockWaits.Done()
		if err != nil {
			db.dropLease(off)
			return nil, err
		}
		return &Lease{db: db, off: off}, nil
	case <-ctx.Done():
	}

	// Release the byte once the abandoned lock returns. The lock belongs to
	// the handle, so the claim stays until then or another waiter here would
	// share the grant.
	go func() {
		defer db.lockWaits.Done()
		if err := <-locked; err == nil {
			_ = db.s.unlock(off)
		}
		db.dropLease(off)
	}()
	return nil, ctx.Err()
}

// claimLeaseLocked claims the lock byte off in this handle, reporting false
// while a lease of the handle holds it. The caller holds bcast.
func (db *DB) claimLeaseLocked(off int64) (bool, error) {
	if db.closed.Load() {
		return false, ErrClosed
	}
	if _, held := db.leases[off]; held {
		return false, nil
	}
	db.leases[off] = struct{}{}
	return true, nil
}

// dropLease drops the claim on off and wakes this handle's waiters.
func (db *DB) dropLease(off int64) {
	db.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(db.leases, off)
		broadcast()
	})
}

// Release releases the lease. It unlocks the byte before waking this
// handle's waiters, so a woken waiter finds it free.
func (l *Lease) Release() error {
	l.once.Do(func() {
		l.err = l.db.s.unlock(l.off)
		l.db.dropLease(l.off)
	})
	return l.err
}
