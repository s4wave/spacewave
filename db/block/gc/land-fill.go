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
	// Name each entry and find the blocks it gives birth to and the
	// recorded holes it refills.
	var born, refills []*block.PutBatchEntry
	err := classifyPuts(ctx, rec, blocks, entries, func(entry *block.PutBatchEntry, kind putKind) {
		switch kind {
		case putBorn:
			born = append(born, entry)
		case putRefill:
			refills = append(refills, entry)
		}
	})
	if err != nil {
		return nil, err
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
		if err := rec.tx.Delete(ctx, rec.key(kindHole, BlockKey(entry.Ref))); err != nil {
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
	return existed, markFillPending(ctx, rec, born)
}

// markFillPending writes a suspectFillPending record for each block in born,
// the blocks a fill gave birth to, that names a child with a suspect record or
// a born block it marks, so a hold onto it promotes it first, as holdStored
// describes. A fill serves a read, so it refuses no block; its own up pairs
// onto such a child hold it only until the fill record nets. It reads each
// child's suspect record only while the Volume may have a counted one, and
// writes no state, so a fill still conflicts with no commit's state write.
// Each pass checks a block's children again, reading each suspect record
// once.
func markFillPending(ctx context.Context, rec *Records, born []*block.PutBatchEntry) error {
	// Without a counted suspect record every child is clean.
	if rec.suspectsKnown && rec.suspectLists == 0 {
		return nil
	}

	// Mark until no born block gains a mark, since a born child may come
	// after its parent in the batch.
	marked := make(map[string]bool, len(born))
	stored := make(map[string]bool)
	for changed := true; changed; {
		changed = false
		for _, entry := range born {
			key := string(BlockKey(entry.Ref))
			if marked[key] {
				continue
			}
			for _, child := range entry.Refs {
				ck := BlockKey(child)
				pending := marked[string(ck)]
				if !pending {
					var read bool
					if pending, read = stored[string(ck)]; !read {
						trust, err := rec.getTrust(ctx, ck)
						if err != nil {
							return err
						}
						pending = trust != listClean
						stored[string(ck)] = pending
					}
				}
				if pending {
					marked[key], changed = true, true
					break
				}
			}
		}
	}
	for key := range marked {
		if err := rec.tx.Set(ctx, rec.key(kindSuspect, []byte(key)), []byte{suspectFillPending}); err != nil {
			return err
		}
	}
	return nil
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
	// Write the record of the block's own implied holder, and the children's
	// up pairs for the collector to apply first.
	key := BlockKey(ref)
	var ups []byte
	if len(children) != 0 {
		ups = EncodeChildren(children)
	}
	return r.writeFill(ctx, key, &JournalRecord{AtUnixMilli: now.UnixMilli(), Down: [][]byte{key}}, ups)
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
