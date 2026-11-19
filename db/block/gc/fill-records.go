package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// A cache fill once wrote the pairs of each block it gave birth to into a
// fill record of its own: an up pair for each child its list names and a
// down pair for its own implied holder. Fills now land through Land under a
// leased root, or through RefillBlocks for a block the records count, and
// write no fill record. The collector still applies and nets the records an
// older build left, by the helpers below: Collector.Collect applies a
// record's up pairs in its first slice and nets its down pair once the
// record is Grace old.

// fillRecordKey returns the key of the fill record of ref stamped at.
func (r *Records) fillRecordKey(at int64, ref []byte) []byte {
	return r.key(kindFillRecord, binary.BigEndian.AppendUint64(nil, uint64(at)), ref) //nolint:gosec // times after 1970.
}

// fillBornKey returns the key of the fill-born index entry of the record of
// the block at key stamped at.
func (r *Records) fillBornKey(key []byte, at int64) []byte {
	return r.key(kindFillBorn, key, binary.BigEndian.AppendUint64(nil, uint64(at))) //nolint:gosec // times after 1970.
}

// writeFill writes fr, the fill record of the block at filled, at its
// stamp, with its fill-born index entry and, unless empty, its up pairs.
func (r *Records) writeFill(ctx context.Context, filled []byte, fr *JournalRecord, ups []byte) error {
	// Write the record and its index entry, then the up pairs.
	value, err := fr.MarshalVT()
	if err != nil {
		return err
	}
	at := fr.GetAtUnixMilli()
	if err := r.tx.Set(ctx, r.fillRecordKey(at, filled), value); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillBornKey(filled, at), nil); err != nil {
		return err
	}
	if len(ups) == 0 {
		return nil
	}
	return r.tx.Set(ctx, r.fillUpsKey(at, filled), ups)
}

// fillUpsKey returns the key of the up pairs of the fill record of the block
// at key stamped at.
func (r *Records) fillUpsKey(at int64, key []byte) []byte {
	return r.key(kindFillUps, binary.BigEndian.AppendUint64(nil, uint64(at)), key) //nolint:gosec // times after 1970.
}

// scanFillUps calls fn with the key and children of each fill record's up
// pairs not applied yet, until fn returns false or an error.
func (r *Records) scanFillUps(ctx context.Context, fn func(key []byte, children []*block.BlockRef) (bool, error)) error {
	prefix := r.key(kindFillUps)
	return r.scan(ctx, prefix, nil, func(key []byte, value func() ([]byte, error)) (bool, error) {
		children, err := decodeValue(value)
		if err != nil {
			return false, err
		}
		return fn(append(bytes.Clone(prefix), key...), children)
	})
}

// fillRecord is one fill record as the collector reads it.
type fillRecord struct {
	// key is the record's key.
	key []byte
	// filled is the block key of the block the fill gave birth to.
	filled []byte
	// rec is the record.
	rec *JournalRecord
}

// scanFillRecords calls fn with each fill record stamped at from or later,
// in time order, until fn returns false or an error.
func (r *Records) scanFillRecords(ctx context.Context, from int64, fn func(fr fillRecord) (bool, error)) error {
	prefix := r.key(kindFillRecord)
	seek := binary.BigEndian.AppendUint64(nil, uint64(from)) //nolint:gosec // times after 1970.
	return r.scan(ctx, prefix, seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Decode the record and hand it to fn.
		if len(key) <= 8 {
			return false, errors.New("fill record key is malformed")
		}
		fr, err := decodeMsg[JournalRecord](value)
		if err != nil {
			return false, err
		}
		return fn(fillRecord{key: append(bytes.Clone(prefix), key...), filled: bytes.Clone(key[8:]), rec: fr})
	})
}

// deleteFillRecord deletes a fill record with its fill-born index entry and
// its up pairs, when the collector has not applied them, so nothing of a
// fill outlives its record.
func (r *Records) deleteFillRecord(ctx context.Context, fr fillRecord) error {
	at := fr.rec.GetAtUnixMilli()
	if err := r.tx.Delete(ctx, r.fillUpsKey(at, fr.filled)); err != nil {
		return err
	}
	if err := r.tx.Delete(ctx, r.fillBornKey(fr.filled, at)); err != nil {
		return err
	}
	return r.tx.Delete(ctx, fr.key)
}

// deleteFills deletes the oldest limit fill records, each as
// deleteFillRecord does, and returns how many it deleted.
func (r *Records) deleteFills(ctx context.Context, limit int) (int, error) {
	// Take the records, then delete them.
	var taken []fillRecord
	err := r.scanFillRecords(ctx, 0, func(fr fillRecord) (bool, error) {
		taken = append(taken, fr)
		return len(taken) < limit, nil
	})
	if err != nil {
		return 0, err
	}
	for _, fr := range taken {
		if err := r.deleteFillRecord(ctx, fr); err != nil {
			return 0, err
		}
	}
	return len(taken), nil
}

// restampFillRecord moves a fill record stamped after now to now, with its
// fill-born index entry and its up pairs not applied yet, so it ages from
// the time the collector first saw it.
func (r *Records) restampFillRecord(ctx context.Context, fr fillRecord, now int64) error {
	// Read the up pairs not applied yet, delete the record at its old time,
	// and write it again at now.
	ups, _, err := r.tx.Get(ctx, r.fillUpsKey(fr.rec.GetAtUnixMilli(), fr.filled))
	if err != nil {
		return err
	}
	ups = bytes.Clone(ups)
	if err := r.deleteFillRecord(ctx, fr); err != nil {
		return err
	}
	moved := fr.rec.CloneVT()
	moved.AtUnixMilli = now
	return r.writeFill(ctx, fr.filled, moved, ups)
}

// fillBorn reports whether a fill record not netted yet gave birth to ref.
func (r *Records) fillBorn(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.anyKey(ctx, r.key(kindFillBorn, BlockKey(ref)))
}
