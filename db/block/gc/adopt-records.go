package block_gc

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// MarkAdoptBorn records that Land gave birth to ref while the Volume adopts,
// so the adoption, which counts the Volume as it was when it began, skips the
// child list Land wrote and counts no holder Land journaled twice.
func (r *Records) MarkAdoptBorn(ctx context.Context, ref *block.BlockRef) error {
	return r.tx.Set(ctx, r.key(kindAdoptBorn, BlockKey(ref)), nil)
}

// AdoptBorn reports whether Land gave birth to ref while the Volume adopts.
func (r *Records) AdoptBorn(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return r.tx.Exists(ctx, r.key(kindAdoptBorn, BlockKey(ref)))
}

// NoteAdoptRoot records, the first time Land moves the root named name in
// bucketID while the Volume adopts, the block it held when the adoption
// began: held, or nil when the name was free.
func (r *Records) NoteAdoptRoot(ctx context.Context, bucketID, name string, held *block.BlockRef) error {
	// Keep the first move's record: it holds the adoption's starting value.
	key := r.namedKey(kindAdoptRoot, bucketID, name)
	found, err := r.tx.Exists(ctx, key)
	if err != nil || found {
		return err
	}
	return r.tx.Set(ctx, key, BlockKey(held))
}

// adoptRecord is one record a bounded adoption scan read.
type adoptRecord struct {
	// key is the record key after its prefix and kind.
	key []byte
	// value is the record value.
	value []byte
}

// scanAfter returns up to limit records of kind whose key, after the prefix
// and kind, sorts after after, in key order; a nil after starts at the first.
// It reads them all before returning, so the caller may write in the same
// transaction.
func (r *Records) scanAfter(ctx context.Context, kind byte, after []byte, limit int) ([]adoptRecord, error) {
	// Position the sorted iterator past after.
	prefix := r.key(kind)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := seekPast(it, r.key(kind, after), after == nil); err != nil {
		return nil, err
	}

	// Copy out the records.
	var out []adoptRecord
	for ; it.Valid() && len(out) < limit; it.Next() {
		value, err := it.Value()
		if err != nil {
			return nil, err
		}
		out = append(out, adoptRecord{key: bytes.Clone(it.Key()[len(prefix):]), value: bytes.Clone(value)})
	}
	return out, it.Err()
}

// scanNamesAfter returns the up to limit smallest keys, after the prefix and
// kind, that sort after after among the records of all the kinds, distinct
// and in key order.
func (r *Records) scanNamesAfter(ctx context.Context, after []byte, limit int, kinds ...byte) ([][]byte, error) {
	// Read the smallest keys of each kind: the smallest of their union are
	// among them.
	var names [][]byte
	for _, kind := range kinds {
		recs, err := r.scanAfter(ctx, kind, after, limit)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			names = append(names, rec.key)
		}
	}

	// Keep the smallest distinct keys.
	slices.SortFunc(names, bytes.Compare)
	names = slices.CompactFunc(names, bytes.Equal)
	return names[:min(len(names), limit)], nil
}

// seekPast positions it at its first key when first is set, and otherwise at
// the first key after key.
func seekPast(it kvtx.Iterator, key []byte, first bool) error {
	if first {
		it.Next()
		return nil
	}
	return it.Seek(append(key, 0))
}
