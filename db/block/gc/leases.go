package block_gc

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
)

// moveLease replaces the lease index record of the root named name in
// bucketID, read from its stored entry, with one at expires, or none at zero.
func (r *Records) moveLease(ctx context.Context, bucketID, name string, expires int64) error {
	// Read the stored expiry.
	old, err := r.GetRoot(ctx, bucketID, name)
	if err != nil {
		return err
	}
	was := old.GetExpiresUnixMilli()
	if was == expires {
		return nil
	}

	// Delete the old record and write the new one.
	if was != 0 {
		if err := r.tx.Delete(ctx, r.leaseKey(was, bucketID, name)); err != nil {
			return err
		}
	}
	if expires == 0 {
		return nil
	}
	return r.tx.Set(ctx, r.leaseKey(expires, bucketID, name), nil)
}

// leaseKey returns the lease index key of the root named name in bucketID at
// expires.
func (r *Records) leaseKey(expires int64, bucketID, name string) []byte {
	at := binary.BigEndian.AppendUint64(nil, uint64(expires)) //nolint:gosec // expiries are positive.
	return r.key(kindLease, at, binary.AppendUvarint(nil, uint64(len(bucketID))), []byte(bucketID), []byte(name))
}

// LapsedRoot is a leased root whose lease lapsed.
type LapsedRoot struct {
	// BucketID is the root's bucket.
	BucketID string
	// Name is the root's name.
	Name string
	// Expires is the lease's expiry in Unix milliseconds.
	Expires int64
}

// ScanLapsed returns up to limit leased roots whose lease lapsed at or before
// now and at or after from, soonest first, reading only their index records.
// It seeks to from, so the deleted index records of the leases a sweep
// released or a renewal moved below from cost the scan nothing.
func (r *Records) ScanLapsed(ctx context.Context, from, now int64, limit int) ([]LapsedRoot, error) {
	var out []LapsedRoot
	at := binary.BigEndian.AppendUint64(nil, uint64(max(from, 0))) //nolint:gosec // clamped to zero.
	err := r.scan(ctx, r.key(kindLease), at, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		// Stop at the limit and at the first lease that has not lapsed.
		if len(out) == limit {
			return false, nil
		}
		if len(key) < 8 {
			return false, errors.New("liveness lease record is malformed")
		}
		expires := int64(binary.BigEndian.Uint64(key)) //nolint:gosec // expiries are positive.
		if expires > now {
			return false, nil
		}
		bucketID, name, err := parseNamedKey(key[8:])
		if err != nil {
			return false, err
		}
		out = append(out, LapsedRoot{BucketID: bucketID, Name: name, Expires: expires})
		return true, nil
	})
	return out, err
}
