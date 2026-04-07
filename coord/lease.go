package coord

import (
	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
)

// coordBucket is the bbolt bucket for coordination state.
var coordBucket = []byte("_coord")

// leaderKey is the well-known key for the leader lease record.
var leaderKey = []byte("leader")

// PutLease writes the leader lease record to the coordination bucket.
func PutLease(tx *bdb.Tx, rec *LeaseRecord) error {
	bkt, err := tx.CreateBucketIfNotExists(coordBucket)
	if err != nil {
		return errors.Wrap(err, "create coord bucket")
	}
	data, err := rec.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal lease record")
	}
	return bkt.Put(leaderKey, data)
}

// GetLease reads the leader lease record. Returns nil if no lease exists.
func GetLease(tx *bdb.Tx) (*LeaseRecord, error) {
	bkt := tx.Bucket(coordBucket)
	if bkt == nil {
		return nil, nil
	}
	data := bkt.Get(leaderKey)
	if data == nil {
		return nil, nil
	}
	rec := new(LeaseRecord)
	if err := rec.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal lease record")
	}
	return rec, nil
}

// DeleteLease removes the leader lease record.
func DeleteLease(tx *bdb.Tx) error {
	bkt := tx.Bucket(coordBucket)
	if bkt == nil {
		return nil
	}
	return bkt.Delete(leaderKey)
}
