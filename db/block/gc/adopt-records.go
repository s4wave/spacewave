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
	// Seek past after, and copy out the records.
	from := after
	if after != nil {
		from = append(bytes.Clone(after), 0)
	}
	var out []adoptRecord
	err := r.scan(ctx, r.key(kind), from, func(key []byte, value func() ([]byte, error)) (bool, error) {
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
