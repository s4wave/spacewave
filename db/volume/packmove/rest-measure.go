package packmove

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// restChunk is the keys one snapshot of MeasureRest counts.
const restChunk = 50000

// MeasureRest counts the keys of src outside the block range and the size of
// those keys and their values, one chunk per snapshot so it runs beside a
// writer. report receives the running totals after each chunk and the final
// ones with Done set. The count is exact only for a source nobody writes.
func MeasureRest(ctx context.Context, src kvtx.Store, keys *store_kvkey.KVKey, report func(RestReport)) (RestReport, error) {
	// Count from the start of the key space, a chunk at a time.
	blocks := keys.GetBlockFullPrefix()
	var total RestReport
	var after []byte
	for !total.Done {
		// Stop when the pack ends, so its error is not held behind the count.
		if err := ctx.Err(); err != nil {
			return total, err
		}
		var chunk RestReport
		var last []byte
		err := readStable(ctx, src, func(tx kvtx.Tx) error {
			var err error
			chunk, last, err = countRest(ctx, tx, blocks, after)
			return err
		})
		if err != nil {
			return total, err
		}
		total.Keys += chunk.Keys
		total.Bytes += chunk.Bytes
		total.Done = chunk.Done
		after = last
		if report != nil {
			report(total)
		}
	}
	return total, nil
}

// countRest counts up to restChunk keys of tx after the key after, skipping
// the block range, and returns the last key it passed.
func countRest(ctx context.Context, tx kvtx.Tx, blocks, after []byte) (RestReport, []byte, error) {
	// Start after the last key counted.
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()
	if err := it.Seek(after); err != nil {
		return RestReport{}, nil, err
	}
	if after != nil && it.Valid() && bytes.Equal(it.Key(), after) {
		it.Next()
	}

	// Count the keys outside the blocks, seeking past the block range.
	var chunk RestReport
	last := after
	for it.Valid() && chunk.Keys < restChunk {
		key := it.Key()
		if bytes.HasPrefix(key, blocks) {
			end := bytes.Clone(blocks)
			end[len(end)-1]++
			if err := it.Seek(end); err != nil {
				return RestReport{}, nil, err
			}
			continue
		}
		value, err := it.ValueCopy(nil)
		if err != nil {
			return RestReport{}, nil, err
		}
		chunk.Keys++
		chunk.Bytes += uint64(len(key) + len(value))
		last = bytes.Clone(key)
		it.Next()
	}
	if err := it.Err(); err != nil {
		return RestReport{}, nil, err
	}
	chunk.Done = !it.Valid()
	return chunk, last, nil
}
