package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
)

// A lease holds a set of named roots for as long as its holder renews it. A
// process holds its reader pins and stage roots under one lease, and a cache
// fill holds the blocks it gives birth to under a lease of its own that it
// never renews. The lease record keeps the expiry, so a renewal writes one
// record however many roots the lease holds, and an index of the roots by
// lease lets the collector release every root of a lapsed lease in slices.
//
// A lease is live while its record holds an expiry after now. It is released
// once its record holds the expiry zero, and ended once the record is gone.
// The records keep these invariants:
//
//  1. A root references a lease only while the lease's record exists: Land
//     writes a root onto a lease only while the lease is live, in the
//     transaction that reads it so, and the collector deletes a lease's record
//     only in the slice that finds no root under it.
//  2. A released lease never lives again: the collector marks a lapsed
//     lease released, expiry zero, in the transaction that reads it lapsed
//     and releases its first root, so the mark precedes every root the
//     collector releases for it, and neither RenewLease nor Land accepts a
//     released or missing lease. A lease that lapsed but is not yet released
//     may still be renewed by a holder whose clock or admission lagged, and
//     the collector then finds it live again.
//  3. So the collector never releases a root under a live lease: it releases
//     only the roots of a released lease, and a lease reaches released only
//     from lapsed, in the collector's own commit.
//  4. Each root of a released lease is released once: the release deletes the
//     root and its index record in one commit, and the next slice lists only
//     the index records left.
//  5. A renewal writes the one lease record, and the release of a lease
//     writes per root what releasing the root by hand would, in slices.
//  6. A lease ID identifies one lease for good: each is a new ULID, so a
//     process that starts again, or whose lease lapsed while it stalled, holds
//     its roots under a new lease and never inherits a lease that lapsed.

// ErrLeaseLapsed is returned by Land for a move onto a lease that is not
// live: its expiry passed, the collector released it, or it never existed.
// The holder renews under a new lease, holding its roots again, and retries.
var ErrLeaseLapsed = errors.New("the lease of a root lapsed")

// GetLease returns the expiry of the lease id in Unix milliseconds, zero for
// a released one, and whether its record exists.
func (r *Records) GetLease(ctx context.Context, id string) (int64, bool, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindLease, []byte(id)))
	if err != nil || !found {
		return 0, false, err
	}
	expires, err := parseLeaseExpiry(value)
	return expires, true, err
}

// LeaseLive reports whether the lease id is live at now, in Unix
// milliseconds: its record holds an expiry after now.
func (r *Records) LeaseLive(ctx context.Context, id string, now int64) (bool, error) {
	expires, _, err := r.GetLease(ctx, id)
	return expires > now, err
}

// StartLease writes the record of the new lease id with the expiry expires,
// in Unix milliseconds. id must be a new ULID, as invariant 6 requires.
func (r *Records) StartLease(ctx context.Context, id string, expires int64) error {
	if id == "" || expires <= 0 {
		return errors.New("a lease needs an ID and an expiry")
	}
	return r.setLease(ctx, id, expires)
}

// RenewLease moves the expiry of the lease id to expires, in Unix milliseconds,
// when the lease is live at now, and reports whether it was. It never renews a
// released lease, whose roots the collector may be releasing, nor one lapsed at
// the caller's now. The holder starts a new lease instead. A caller whose now
// lags may renew a lease the collector's clock finds lapsed, which is safe
// until the collector marks it released. Cost: one point read and one write.
func (r *Records) RenewLease(ctx context.Context, id string, now, expires int64) (bool, error) {
	live, err := r.LeaseLive(ctx, id, now)
	if err != nil || !live {
		return false, err
	}
	return true, r.setLease(ctx, id, expires)
}

// setLease writes the lease record of id with expires.
func (r *Records) setLease(ctx context.Context, id string, expires int64) error {
	return r.tx.Set(ctx, r.key(kindLease, []byte(id)), binary.BigEndian.AppendUint64(nil, uint64(expires))) //nolint:gosec // expiries are not negative.
}

// parseLeaseExpiry decodes the value of a lease record.
func parseLeaseExpiry(value []byte) (int64, error) {
	if len(value) != 8 {
		return 0, errors.New("liveness lease record is malformed")
	}
	return int64(binary.BigEndian.Uint64(value)), nil //nolint:gosec // written from a non-negative int64.
}

// rootExpiry returns the expiry of entry's root in Unix milliseconds: zero
// for a root without a lease, its lease's expiry for a live or lapsed one,
// and 1, long past, for a lease the collector released or ended. leases
// caches the expiry of each lease read, so a walk reads each lease once.
func (r *Records) rootExpiry(ctx context.Context, entry *RootEntry, leases map[string]int64) (int64, error) {
	// A root without a lease never lapses.
	id := entry.GetLease()
	if id == "" {
		return 0, nil
	}

	// Read the lease once per walk.
	expires, ok := leases[id]
	if !ok {
		var err error
		if expires, _, err = r.GetLease(ctx, id); err != nil {
			return 0, err
		}
		expires = max(expires, 1)
		leases[id] = expires
	}
	return expires, nil
}

// LapsedLeases returns up to limit leases that are not live at now, in Unix
// milliseconds, from the lease from on, in ID order. It reads each lease
// record from from on: there is one per running or recently stopped process
// and one per fill window of the last grace period.
func (r *Records) LapsedLeases(ctx context.Context, from string, now int64, limit int) ([]LapsedLease, error) {
	var out []LapsedLease
	err := r.scanKind(ctx, kindLease, []byte(from), func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Skip a live lease.
		v, err := value()
		if err != nil {
			return false, err
		}
		expires, err := parseLeaseExpiry(v)
		if err != nil || expires > now {
			return err == nil, err
		}

		// Keep a lapsed lease, up to the limit.
		out = append(out, LapsedLease{ID: string(key), Expires: expires})
		return len(out) < limit, nil
	})
	return out, err
}

// ListLeaseRoots returns up to limit roots the lease id holds, reading only
// their index records.
func (r *Records) ListLeaseRoots(ctx context.Context, id string, limit int) ([]LeasedRoot, error) {
	var out []LeasedRoot
	if limit <= 0 {
		return nil, nil
	}
	err := r.scan(ctx, r.leaseRootPrefix(id), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		bucketID, name, err := ParseNamedKey(key)
		if err != nil {
			return false, err
		}
		out = append(out, LeasedRoot{BucketID: bucketID, Name: name})
		return len(out) < limit, nil
	})
	return out, err
}

// ReleaseLease marks the lease id released, so Land refuses a root onto it
// and no renewal brings it back, ahead of the release of its roots.
func (r *Records) ReleaseLease(ctx context.Context, id string) error {
	return r.setLease(ctx, id, 0)
}

// EndLease deletes the record of the lease id, which holds no root.
func (r *Records) EndLease(ctx context.Context, id string) error {
	return r.tx.Delete(ctx, r.key(kindLease, []byte(id)))
}

// moveLeaseRoot moves the lease index record of the root named name in bucketID
// from the lease its stored entry references to lease, none for empty.
func (r *Records) moveLeaseRoot(ctx context.Context, bucketID, name, lease string) error {
	// Read the stored lease.
	old, err := r.GetRoot(ctx, bucketID, name)
	if err != nil {
		return err
	}
	was := old.GetLease()
	if was == lease {
		return nil
	}

	// Delete the old record and write the new one.
	if was != "" {
		if err := r.tx.Delete(ctx, r.leaseRootKey(was, bucketID, name)); err != nil {
			return err
		}
	}
	if lease == "" {
		return nil
	}
	return r.tx.Set(ctx, r.leaseRootKey(lease, bucketID, name), nil)
}

// deleteLeaseRoot deletes the index record of the root named name in bucketID
// under the lease id, one whose root no longer references the lease.
func (r *Records) deleteLeaseRoot(ctx context.Context, id, bucketID, name string) error {
	return r.tx.Delete(ctx, r.leaseRootKey(id, bucketID, name))
}

// leaseRootPrefix returns the prefix of the lease index records of the roots
// the lease id holds.
func (r *Records) leaseRootPrefix(id string) []byte {
	return r.key(kindLeaseRoot, binary.AppendUvarint(nil, uint64(len(id))), []byte(id))
}

// leaseRootKey returns the lease index key of the root named name in
// bucketID under the lease id.
func (r *Records) leaseRootKey(id, bucketID, name string) []byte {
	return r.key(
		kindLeaseRoot,
		binary.AppendUvarint(nil, uint64(len(id))), []byte(id),
		binary.AppendUvarint(nil, uint64(len(bucketID))), []byte(bucketID), []byte(name),
	)
}

// CarryFormat10Leases moves up to limit of the leased roots a format 10 or
// older store indexed by expiry under its retired kind 'E' onto a new lease,
// and deletes their old index records. The lease expires at the largest
// expiry the slice carries, so a root another process still held when the
// upgrade ran lapses no sooner than it would have, and once it lapses the
// collector releases its roots in slices as it does any other lease's. Each
// slice starts a lease of its own, so a rerun after a crash never revives a
// carried lease that a collector already released.
//
// The upgrade runs at open, before this process holds a root. Only s4db refuses
// it while another process shares the store, and only through a check made
// before the open, which another process may race. SQLite, which several
// processes share through its file lock, and redis refuse nothing, so there an
// old process may still hold carried roots. The carried expiry keeps them for
// it until it would have lapsed.
//
// The rewrite takes each entry whole, so the retired expiry field goes with
// the record. It moves no holder: each root keeps its block. It returns
// whether more may follow. Each call works in the caller's transaction, so a
// rerun after a crash carries the records left.
func (r *Records) CarryFormat10Leases(ctx context.Context, limit int) (bool, error) {
	// Read a run of the old index records, each with the root it indexes.
	type old struct {
		key            []byte
		bucketID, name string
	}

	// Scan up to limit of them, with the largest expiry they hold.
	var run []old
	var expires int64
	err := r.scanKind(ctx, format10Lease, nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Read the expiry and the root's bucket and name.
		if len(key) < 8 {
			return false, errors.New("format 10 lease record is malformed")
		}
		expires = max(expires, int64(binary.BigEndian.Uint64(key[:8]))) //nolint:gosec // written from a non-negative int64.
		bucketID, name, err := ParseNamedKey(key[8:])
		if err != nil {
			return false, err
		}
		run = append(run, old{key: r.key(format10Lease, bytes.Clone(key)), bucketID: bucketID, name: name})
		return len(run) < limit, nil
	})
	if err != nil || len(run) == 0 {
		return false, err
	}

	// Start the slice's lease, then move each root still held onto it. An
	// expiry of zero would mark the lease released, so it holds at least 1.
	id := ulid.NewULID()
	if err := r.StartLease(ctx, id, max(expires, 1)); err != nil {
		return false, err
	}
	for _, o := range run {
		if err := r.tx.Delete(ctx, o.key); err != nil {
			return false, err
		}
		entry, err := r.GetRoot(ctx, o.bucketID, o.name)
		if err != nil {
			return false, err
		}
		if entry == nil {
			continue
		}
		carried := &RootEntry{Ref: entry.GetRef(), Lease: id}
		if err := r.SetRoot(ctx, o.bucketID, o.name, carried); err != nil {
			return false, err
		}
	}
	return len(run) == limit, nil
}
