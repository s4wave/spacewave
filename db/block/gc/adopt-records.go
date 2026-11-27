package block_gc

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

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

// adoptRootStart returns the block key the root whose namedKey after its
// kind is name held when the adoption began, empty when the name was free,
// and whether Land moved the name since, so NoteAdoptRoot recorded it.
func (r *Records) adoptRootStart(ctx context.Context, name []byte) ([]byte, bool, error) {
	return r.tx.Get(ctx, r.key(kindAdoptRoot, name))
}

// adoptRecord is one record a bounded adoption scan read.
type adoptRecord struct {
	// key is the record key after its prefix and kind.
	key []byte
	// value is the record value.
	value []byte
}

// scanAfter returns up to limit records of kind whose key, after the prefix
// and kind, sorts after after, in key order. A nil after starts at the first
// record. It reads them all before returning, so the caller may write in the
// same transaction.
func (r *Records) scanAfter(ctx context.Context, kind byte, after []byte, limit int) ([]adoptRecord, error) {
	// Seek past after, and copy out the records.
	from := after
	if after != nil {
		from = append(bytes.Clone(after), 0)
	}
	var out []adoptRecord
	err := r.scanKind(ctx, kind, from, func(key []byte, value func() ([]byte, error)) (bool, error) {
		if len(out) == limit {
			return false, nil
		}
		v, err := value()
		out = append(out, adoptRecord{key: bytes.Clone(key), value: bytes.Clone(v)})
		return true, err
	})
	return out, err
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

// scanThrough returns the records of kind whose key, after the prefix and
// kind, sorts after after and at or before last, in key order. A nil after
// starts at the first record, and a nil last reads to the kind's end. It reads
// them all before returning, so the caller may write in the same transaction.
func (r *Records) scanThrough(ctx context.Context, kind byte, after, last []byte) (sortedRecords, error) {
	// Seek past after, and copy out the records through last.
	from := after
	if after != nil {
		from = append(bytes.Clone(after), 0)
	}
	var out sortedRecords
	err := r.scanKind(ctx, kind, from, func(key []byte, value func() ([]byte, error)) (bool, error) {
		if last != nil && bytes.Compare(key, last) > 0 {
			return false, nil
		}
		v, err := value()
		out = append(out, adoptRecord{key: bytes.Clone(key), value: bytes.Clone(v)})
		return true, err
	})
	return out, err
}

// sortedRecords is the records of a range scan in key order, which find
// walks once beside keys asked in ascending order, merging the scan with
// them instead of a point read of each.
type sortedRecords []adoptRecord

// find returns the value of the record at key and whether one exists,
// dropping the records before key. Keys are asked in ascending order.
func (s *sortedRecords) find(key []byte) ([]byte, bool) {
	for len(*s) != 0 && bytes.Compare((*s)[0].key, key) < 0 {
		*s = (*s)[1:]
	}
	if len(*s) != 0 && bytes.Equal((*s)[0].key, key) {
		return (*s)[0].value, true
	}
	return nil, false
}
