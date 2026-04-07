package coord

import (
	"strconv"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
)

// participantsBucket is the bbolt bucket name for participant records.
var participantsBucket = []byte("_coord/participants")

// PutParticipant writes a participant record to the registry within a
// bbolt write transaction. The record is keyed by PID.
func PutParticipant(tx *bdb.Tx, rec *ParticipantRecord) error {
	bkt, err := tx.CreateBucketIfNotExists(participantsBucket)
	if err != nil {
		return errors.Wrap(err, "create participants bucket")
	}
	data, err := rec.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal participant record")
	}
	key := []byte(strconv.FormatUint(uint64(rec.GetPid()), 10))
	return bkt.Put(key, data)
}

// GetParticipant reads a participant record by PID from a bbolt
// transaction. Returns nil if the record does not exist.
func GetParticipant(tx *bdb.Tx, pid uint32) (*ParticipantRecord, error) {
	bkt := tx.Bucket(participantsBucket)
	if bkt == nil {
		return nil, nil
	}
	key := []byte(strconv.FormatUint(uint64(pid), 10))
	data := bkt.Get(key)
	if data == nil {
		return nil, nil
	}
	rec := new(ParticipantRecord)
	if err := rec.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal participant record")
	}
	return rec, nil
}

// DeleteParticipant removes a participant record by PID from a bbolt
// write transaction.
func DeleteParticipant(tx *bdb.Tx, pid uint32) error {
	bkt := tx.Bucket(participantsBucket)
	if bkt == nil {
		return nil
	}
	key := []byte(strconv.FormatUint(uint64(pid), 10))
	return bkt.Delete(key)
}

// ListParticipants returns all participant records in the registry.
func ListParticipants(tx *bdb.Tx) ([]*ParticipantRecord, error) {
	bkt := tx.Bucket(participantsBucket)
	if bkt == nil {
		return nil, nil
	}
	var records []*ParticipantRecord
	err := bkt.ForEach(func(k, v []byte) error {
		rec := new(ParticipantRecord)
		if err := rec.UnmarshalVT(v); err != nil {
			return errors.Wrap(err, "unmarshal participant record")
		}
		records = append(records, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}
