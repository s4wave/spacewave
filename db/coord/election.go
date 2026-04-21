//go:build !js

package coord

import (
	"context"
	"os"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
)

// LeaseStaleThreshold is the duration after which a lease is considered stale.
const LeaseStaleThreshold = time.Second

// Election manages leader election via bbolt lease records.
type Election struct {
	db         *bdb.DB
	pid        uint32
	socketPath string
}

// NewElection creates a new election manager.
func NewElection(db *bdb.DB, socketPath string) *Election {
	return &Election{
		db:         db,
		pid:        uint32(os.Getpid()),
		socketPath: socketPath,
	}
}

// TryClaimLeadership attempts to claim leadership if no valid lease exists.
// Returns true if this process became the leader.
func (e *Election) TryClaimLeadership() (bool, error) {
	var claimed bool
	err := e.db.Update(func(tx *bdb.Tx) error {
		lease, err := GetLease(tx)
		if err != nil {
			return err
		}

		if lease != nil && e.isLeaseValid(lease) {
			// Valid lease exists, cannot claim.
			claimed = false
			return nil
		}

		// No valid lease: claim leadership.
		now := time.Now().UnixNano()
		newLease := &LeaseRecord{
			LeaderPid:           e.pid,
			LeaseTimestampNanos: now,
			LeaderSocketPath:    e.socketPath,
		}
		claimed = true
		return PutLease(tx, newLease)
	})
	return claimed, err
}

// isLeaseValid checks if a lease record represents a live, non-stale leader.
func (e *Election) isLeaseValid(lease *LeaseRecord) bool {
	// If the lease holder is us, it's valid (we're re-checking our own lease).
	if lease.GetLeaderPid() == e.pid {
		return true
	}

	// Check if the leader process is alive.
	if !isProcessAlive(int(lease.GetLeaderPid())) {
		return false
	}

	// Check if the lease is fresh (within stale threshold).
	leaseTime := time.Unix(0, lease.GetLeaseTimestampNanos())
	return time.Since(leaseTime) <= LeaseStaleThreshold
}

// CurrentLeader returns the current lease record, or nil if none exists.
func (e *Election) CurrentLeader() (*LeaseRecord, error) {
	var lease *LeaseRecord
	err := e.db.View(func(tx *bdb.Tx) error {
		var readErr error
		lease, readErr = GetLease(tx)
		return readErr
	})
	return lease, err
}

// IsLeader returns true if this process currently holds the lease.
func (e *Election) IsLeader() (bool, error) {
	lease, err := e.CurrentLeader()
	if err != nil {
		return false, err
	}
	return lease != nil && lease.GetLeaderPid() == e.pid, nil
}

// LeaseRenewalInterval is the default interval between lease renewals.
const LeaseRenewalInterval = 250 * time.Millisecond

// RenewLease updates the lease timestamp for the current leader.
// Returns false if this process is no longer the leader.
func (e *Election) RenewLease() (bool, error) {
	var renewed bool
	err := e.db.Update(func(tx *bdb.Tx) error {
		lease, err := GetLease(tx)
		if err != nil {
			return err
		}
		if lease == nil || lease.GetLeaderPid() != e.pid {
			renewed = false
			return nil
		}
		lease.LeaseTimestampNanos = time.Now().UnixNano()
		renewed = true
		return PutLease(tx, lease)
	})
	return renewed, err
}

// RunLeaseRenewal renews the lease periodically until ctx is cancelled or
// this process loses leadership. Returns nil on context cancellation.
func (e *Election) RunLeaseRenewal(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(LeaseRenewalInterval):
			renewed, err := e.RenewLease()
			if err != nil {
				return err
			}
			if !renewed {
				return nil
			}
		}
	}
}

// TryReelect attempts to claim leadership when the current leader appears
// unreachable. Only succeeds if the lease is stale (timestamp older than
// LeaseStaleThreshold) or the leader PID is dead. The bbolt per-txn fcntl
// lock serializes competing re-election attempts from multiple followers.
func (e *Election) TryReelect() (bool, error) {
	var claimed bool
	err := e.db.Update(func(tx *bdb.Tx) error {
		lease, err := GetLease(tx)
		if err != nil {
			return err
		}

		// No lease at all: claim immediately.
		if lease == nil {
			return e.writeLease(tx, &claimed)
		}

		// If we already hold it, nothing to do.
		if lease.GetLeaderPid() == e.pid {
			claimed = true
			return nil
		}

		// Leader PID dead: claim.
		if !isProcessAlive(int(lease.GetLeaderPid())) {
			return e.writeLease(tx, &claimed)
		}

		// Check staleness.
		leaseTime := time.Unix(0, lease.GetLeaseTimestampNanos())
		if time.Since(leaseTime) > LeaseStaleThreshold {
			return e.writeLease(tx, &claimed)
		}

		// Lease is still valid, cannot claim.
		claimed = false
		return nil
	})
	return claimed, err
}

// ReleaseLease deletes the lease record if this process is the leader.
// Called during graceful shutdown so followers detect the missing lease and
// trigger immediate election without waiting for staleness timeout.
func (e *Election) ReleaseLease() error {
	return e.db.Update(func(tx *bdb.Tx) error {
		lease, err := GetLease(tx)
		if err != nil {
			return err
		}
		if lease == nil || lease.GetLeaderPid() != e.pid {
			return nil
		}
		return DeleteLease(tx)
	})
}

// writeLease writes a new lease for this process.
func (e *Election) writeLease(tx *bdb.Tx, claimed *bool) error {
	newLease := &LeaseRecord{
		LeaderPid:           e.pid,
		LeaseTimestampNanos: time.Now().UnixNano(),
		LeaderSocketPath:    e.socketPath,
	}
	*claimed = true
	return PutLease(tx, newLease)
}
