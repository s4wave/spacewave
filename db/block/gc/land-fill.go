package block_gc

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// LandFill stores the entries of a cache fill, which copies blocks a peer
// holds, as Land stores an unheld put: each block it gives birth to gets its
// child list, an up pair for each child the list names and a down pair for
// its own implied holder, so its children are held from the fill on and the
// block is garbage after the grace period unless something holds it. A
// recorded hole is stored, as Land counts it: the fill writes its list back
// and journals nothing. It returns, for each entry, whether blocks held its
// bytes before.
//
// The pairs go to a fill record of their own, not the journal, written blind
// at now: a fill reads only the child list of each block it puts, whether the
// Volume records any hole and, when it does, each block's hole record, so an
// optimistic fill conflicts only with a commit that stores or frees one of
// its blocks or records or refills a hole, never with the journal's sequence
// number.
// Collector.Collect nets each aged fill record whole.
func LandFill(ctx context.Context, rec *Records, blocks block.StoreOps, now time.Time, entries []*block.PutBatchEntry) ([]bool, error) {
	// Name each entry and find the blocks without a child list, and the
	// recorded holes it refills.
	born := make(map[string]*block.PutBatchEntry)
	holes := holeReader{rec: rec}
	var refills []*block.PutBatchEntry
	for _, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return nil, block.ErrEmptyBlock
		}
		if !entry.RefsKnown {
			return nil, errors.Wrap(block.ErrRefsUnknown, entry.Ref.MarshalString())
		}
		if entry.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: blocks.GetHashType()})
			if err != nil {
				return nil, err
			}
			entry.Ref = ref
		}
		key := string(BlockKey(entry.Ref))
		if _, ok := born[key]; ok {
			continue
		}
		listed, err := rec.HasChildren(ctx, entry.Ref)
		if err != nil || listed {
			if err != nil {
				return nil, err
			}
			continue
		}
		hole, err := holes.isHole(ctx, entry.Ref)
		if err != nil {
			return nil, err
		}
		if hole {
			refills = append(refills, entry)
			continue
		}
		born[key] = entry
	}

	// Store the bytes, write the refilled holes' lists back, then write each
	// born block's child list and fill record.
	existed, err := blocks.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range refills {
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			return nil, err
		}
		if err := rec.DeleteHole(ctx, entry.Ref); err != nil {
			return nil, err
		}
	}
	for _, entry := range born {
		children := normalizeChildren(entry.Refs)
		if err := rec.PutChildren(ctx, entry.Ref, children); err != nil {
			return nil, err
		}
		if err := rec.putFillRecord(ctx, entry.Ref, children, now); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// fillRecordKey returns the key of the fill record of ref stamped at.
func (r *Records) fillRecordKey(at int64, ref []byte) []byte {
	return r.key(kindFillRecord, binary.BigEndian.AppendUint64(nil, uint64(at)), ref) //nolint:gosec // times after 1970.
}

// fillBornKey returns the key of the fill-born index entry of the record of
// the block at key stamped at.
func (r *Records) fillBornKey(key []byte, at int64) []byte {
	return r.key(kindFillBorn, key, binary.BigEndian.AppendUint64(nil, uint64(at))) //nolint:gosec // times after 1970.
}

// putFillRecord writes the fill record of ref, which names children, with its
// fill-born index entry and the hold index entry of each child, all blind.
func (r *Records) putFillRecord(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef, now time.Time) error {
	// Write the record.
	key := BlockKey(ref)
	fr := &JournalRecord{AtUnixMilli: now.UnixMilli(), Down: [][]byte{key}}
	for _, child := range children {
		fr.Up = append(fr.Up, BlockKey(child))
	}
	value, err := fr.MarshalVT()
	if err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillRecordKey(fr.GetAtUnixMilli(), key), value); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillBornKey(key, fr.GetAtUnixMilli()), nil); err != nil {
		return err
	}

	// Index each up pair by the child it holds.
	for _, up := range fr.GetUp() {
		if err := r.tx.Set(ctx, r.key(kindFillHold, up, key), nil); err != nil {
			return err
		}
	}
	return nil
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

// scanFillRecords calls fn with each fill record in time order, until fn
// returns false or an error.
func (r *Records) scanFillRecords(ctx context.Context, fn func(fr fillRecord) (bool, error)) error {
	return r.scanFillRecordsFrom(ctx, 0, fn)
}

// scanFillRecordsFrom calls fn with each fill record stamped at from or
// later, in time order, until fn returns false or an error.
func (r *Records) scanFillRecordsFrom(ctx context.Context, from int64, fn func(fr fillRecord) (bool, error)) error {
	// Walk the records in key order, which is time order, from from.
	prefix := r.key(kindFillRecord)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(r.fillRecordKey(from, nil)); err != nil {
		return err
	}
	for ; it.Valid(); it.Next() {
		// Decode the record.
		key := it.Key()
		if len(key) <= len(prefix)+8 {
			return errors.New("fill record key is malformed")
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		fr := &JournalRecord{}
		if err := fr.UnmarshalVT(value); err != nil {
			return err
		}

		// Hand it to fn.
		more, err := fn(fillRecord{key: append([]byte(nil), key...), filled: append([]byte(nil), key[len(prefix)+8:]...), rec: fr})
		if err != nil || !more {
			return err
		}
	}
	return it.Err()
}

// deleteFillRecord deletes a fill record with its fill-born index entry and
// the hold index entries of its up pairs, so no index entry outlives its
// record.
func (r *Records) deleteFillRecord(ctx context.Context, fr fillRecord) error {
	for _, up := range fr.rec.GetUp() {
		if err := r.tx.Delete(ctx, r.key(kindFillHold, up, fr.filled)); err != nil {
			return err
		}
	}
	if err := r.tx.Delete(ctx, r.fillBornKey(fr.filled, fr.rec.GetAtUnixMilli())); err != nil {
		return err
	}
	return r.tx.Delete(ctx, fr.key)
}

// restampFillRecord moves a fill record stamped after now to now, with its
// index entries, so it ages from the time the collector first saw it.
func (r *Records) restampFillRecord(ctx context.Context, fr fillRecord, now int64) error {
	// Delete the record at its old time.
	if err := r.deleteFillRecord(ctx, fr); err != nil {
		return err
	}

	// Write it again at now.
	moved := fr.rec.CloneVT()
	moved.AtUnixMilli = now
	value, err := moved.MarshalVT()
	if err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillRecordKey(now, fr.filled), value); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillBornKey(fr.filled, now), nil); err != nil {
		return err
	}
	for _, up := range moved.GetUp() {
		if err := r.tx.Set(ctx, r.key(kindFillHold, up, fr.filled), nil); err != nil {
			return err
		}
	}
	return nil
}

// fillBorn reports whether a fill record not netted yet gave birth to ref.
func (r *Records) fillBorn(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.anyKey(ctx, r.key(kindFillBorn, BlockKey(ref)))
}

// fillHeld reports whether a fill record not netted yet holds ref.
func (r *Records) fillHeld(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.anyKey(ctx, r.key(kindFillHold, BlockKey(ref)))
}
