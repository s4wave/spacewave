package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"

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
//  1. A root names a lease only while the lease's record exists: Land writes
//     a root onto a lease only while the lease is live, in the transaction
//     that reads it so, and the collector deletes a lease's record only in
//     the slice that finds no root under it.
//  2. A lease that lapsed never lives again: RenewLease renews only a live
//     lease, and the collector marks a lapsed lease released in the
//     transaction of its first release, so the mark precedes every root the
//     collector releases for it, and Land refuses a root onto it.
//  3. So the collector never releases a root under a live lease: it releases
//     only the roots of a released lease, and a lease reaches released only
//     from lapsed, in the collector's own commit.
//  4. Each root of a released lease is released once: the release deletes the
//     root and its index record in one commit, and the next slice lists only
//     the index records left.
//  5. A renewal writes the one lease record, and the release of a lease
//     writes per root what releasing the root by hand would, in slices.
//  6. A lease ID names one lease for good: each is a new ULID, so a process
//     that starts again, or whose lease lapsed while it stalled, holds its
//     roots under a new lease and never inherits a lease that lapsed.

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

// RenewLease moves the expiry of the lease id to expires, in Unix
// milliseconds, when the lease is live at now, and reports whether it was.
// It never renews a lapsed or released lease, whose roots the collector may
// be releasing; the holder starts a new lease instead. Cost: one point read
// and one write.
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
	err := r.scan(ctx, r.key(kindLease), []byte(from), func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Skip a live lease.
		v, err := value()
		if err != nil {
			return false, err
		}
		expires, err := parseLeaseExpiry(v)
		if err != nil || expires > now {
			return err == nil, err
		}

		// Keep a lapsed one, up to the limit.
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

// moveLeaseRoot moves the lease index record of the root named name in
// bucketID from the lease its stored entry names to lease, none for empty.
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

// deleteLeaseRoot deletes the index record of the root named name in
// bucketID under the lease id, one whose root no longer names the lease.
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

// format10LeaseID is the lease the format upgrade gives the leased roots of
// a format 10 or older store, lapsed from the start: the upgrade runs at
// open, before this process holds a root, and a store such as s4db refuses
// it while another process shares the store, so no live process holds them.
const format10LeaseID = "format10"

// format10LeaseExpiry is the expiry of format10LeaseID: lapsed at once, and
// the same on every rerun of the upgrade.
const format10LeaseExpiry = 1

// CarryFormat10Leases moves up to limit of the leased roots a format 10 or
// older store indexed by expiry under its retired kind 'E' onto
// format10LeaseID, a lapsed lease the collector then releases in slices as
// it does any other, and deletes their old index records. It rewrites each
// entry whole, so the retired expiry field goes with the record. It moves no
// holder: each root keeps its block. It returns whether more may follow.
// Each call works in the caller's transaction, so a rerun after a crash
// carries the records left.
func (r *Records) CarryFormat10Leases(ctx context.Context, limit int) (bool, error) {
	// Read a run of the old index records, each with the root it indexes.
	type old struct {
		key            []byte
		bucketID, name string
	}

	// Scan up to limit of them.
	var run []old
	err := r.scan(ctx, r.key(format10Lease), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Skip the expiry and read the root's bucket and name.
		if len(key) < 8 {
			return false, errors.New("format 10 lease record is malformed")
		}
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

	// Start the carried lease, then move each root still held onto it.
	if err := r.setLease(ctx, format10LeaseID, format10LeaseExpiry); err != nil {
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
		carried := &RootEntry{Ref: entry.GetRef(), Lease: format10LeaseID}
		if err := r.SetRoot(ctx, o.bucketID, o.name, carried); err != nil {
			return false, err
		}
	}
	return len(run) == limit, nil
}
