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
//
// Until the Volume's old generation settles, a fill keeps the settle's
// barrier as Land does: while Open it pushes onto the front each unverified
// child of a block it gives birth to or refills, and while Sweeping a fill
// of an unmarked block gives birth to it again, as Records.rebirth
// describes, with the old list's children outside the run in the record's
// down pairs. A fill refuses no hold, since it serves a read. Learning the
// generation reads the state record until the transaction knows the Volume
// is Settled, so during the settle a fill also conflicts with a commit that
// writes the state.
func LandFill(ctx context.Context, rec *Records, blocks block.StoreOps, now time.Time, entries []*block.PutBatchEntry) ([]bool, error) {
	// Name each entry and find the blocks it gives birth to and the
	// recorded holes it refills.
	var born, refills []*block.PutBatchEntry
	reborn := make(map[string]bool)
	err := classifyPuts(ctx, rec, blocks, entries, func(entry *block.PutBatchEntry, kind putKind) {
		switch kind {
		case putBorn:
			born = append(born, entry)
		case putReborn:
			born = append(born, entry)
			reborn[string(BlockKey(entry.Ref))] = true
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
		if err := rec.refill(ctx, entry); err != nil {
			return nil, err
		}
	}
	var state *LivenessState
	for _, entry := range born {
		var released [][]byte
		if reborn[string(BlockKey(entry.Ref))] {
			if state == nil {
				if state, err = rec.GetState(ctx); err != nil {
					return nil, err
				}
			}
			if released, err = rec.rebirth(ctx, state, entry.Ref); err != nil {
				return nil, err
			}
		}
		children := normalizeChildren(entry.Refs)
		if err := rec.PutChildren(ctx, entry.Ref, children); err != nil {
			return nil, err
		}
		if err := rec.putFillRecord(ctx, entry.Ref, children, released, now); err != nil {
			return nil, err
		}
	}

	// Write the cascade count a rebirth took over, then keep the front
	// invariant once every list is written, so a born child later in the
	// batch is marked already.
	if state != nil {
		if err := rec.SetState(ctx, state); err != nil {
			return nil, err
		}
	}
	if len(born) == 0 {
		return existed, nil
	}
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return existed, err
	}
	for _, entry := range born {
		if err := rec.pushChildren(ctx, EncodeChildren(entry.Refs), nil); err != nil {
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
func (r *Records) putFillRecord(ctx context.Context, ref *block.BlockRef, children []*block.BlockRef, released [][]byte, now time.Time) error {
	// Write the record of the block's own implied holder, and the children's
	// up pairs for the collector to apply first.
	key := BlockKey(ref)
	var ups []byte
	if len(children) != 0 {
		ups = EncodeChildren(children)
	}
	down := append([][]byte{key}, released...)
	return r.writeFill(ctx, key, &JournalRecord{AtUnixMilli: now.UnixMilli(), Down: down}, ups)
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
