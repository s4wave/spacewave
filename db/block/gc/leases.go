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
}

// ScanLapsed returns up to limit leased roots whose lease lapsed at or before
// now, soonest first, reading only their index records.
func (r *Records) ScanLapsed(ctx context.Context, now int64, limit int) ([]LapsedRoot, error) {
	// Iterate the index in expiry order.
	prefix := r.key(kindLease)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Stop at the first lease that has not lapsed.
	var out []LapsedRoot
	for it.Next(); it.Valid() && len(out) < limit; it.Next() {
		rest := it.Key()[len(prefix):]
		if len(rest) < 8 {
			return nil, errors.New("liveness lease record is malformed")
		}
		if int64(binary.BigEndian.Uint64(rest)) > now { //nolint:gosec // expiries are positive.
			break
		}
		bucketID, name, err := parseNamedKey(rest[8:])
		if err != nil {
			return nil, err
		}
		out = append(out, LapsedRoot{BucketID: bucketID, Name: name})
	}
	return out, it.Err()
}
