package block_gc

import (
	"bytes"
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
// block is garbage after the grace period unless something holds it. The
// collector applies the up pairs in its first slice after the fill, before
// it frees anything, and nets the down pair once the record ages. A
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
// Collector.Collect applies the record's up pairs in its first slice, and
// nets its down pair once the record is Grace old.
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

// putFillRecord writes the fill record of ref, with its fill-born index
// entry and its up pairs for children, all blind.
func (r *Records) putFillRecord(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef, now time.Time) error {
	// Write the record of the block's own implied holder.
	key := BlockKey(ref)
	at := now.UnixMilli()
	fr := &JournalRecord{AtUnixMilli: at, Down: [][]byte{key}}
	value, err := fr.MarshalVT()
	if err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillRecordKey(at, key), value); err != nil {
		return err
	}
	if err := r.tx.Set(ctx, r.fillBornKey(key, at), nil); err != nil {
		return err
	}

	// Write the children's up pairs for the collector to apply first.
	if len(children) == 0 {
		return nil
	}
	return r.tx.Set(ctx, r.fillUpsKey(at, key), EncodeChildren(children))
}

// fillUpsKey returns the key of the up pairs of the fill record of the block
// at key stamped at.
func (r *Records) fillUpsKey(at int64, key []byte) []byte {
	return r.key(kindFillUps, binary.BigEndian.AppendUint64(nil, uint64(at)), key) //nolint:gosec // times after 1970.
}

// scanFillUps calls fn with the key and children of each fill record's up
// pairs not applied yet, until fn returns false or an error.
func (r *Records) scanFillUps(ctx context.Context, fn func(key []byte, children []*block.BlockRef) (bool, error)) error {
	// Walk the fill up records in key order.
	prefix := r.key(kindFillUps)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(prefix); err != nil {
		return err
	}
	for ; it.Valid(); it.Next() {
		value, err := it.Value()
		if err != nil {
			return err
		}
		children, err := DecodeChildren(value)
		if err != nil {
			return err
		}
		more, err := fn(bytes.Clone(it.Key()), children)
		if err != nil || !more {
			return err
		}
	}
	return it.Err()
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

// restampFillRecord moves a fill record stamped after now to now, with its
// fill-born index entry and its up pairs not applied yet, so it ages from
// the time the collector first saw it.
func (r *Records) restampFillRecord(ctx context.Context, fr fillRecord, now int64) error {
	// Read the up pairs not applied yet, then delete the record at its old
	// time.
	ups, found, err := r.tx.Get(ctx, r.fillUpsKey(fr.rec.GetAtUnixMilli(), fr.filled))
	if err != nil {
		return err
	}
	ups = bytes.Clone(ups)
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
	if !found {
		return nil
	}
	return r.tx.Set(ctx, r.fillUpsKey(now, fr.filled), ups)
}

// fillBorn reports whether a fill record not netted yet gave birth to ref.
func (r *Records) fillBorn(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.anyKey(ctx, r.key(kindFillBorn, BlockKey(ref)))
}
