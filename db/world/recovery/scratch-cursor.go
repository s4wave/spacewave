//go:build !js && !wasip1

package world_recovery

import "bytes"

// scratchCursor closes every native keyset read before its caller processes a row.
type scratchCursor struct {
	// bucket selects the disk index and transaction.
	bucket *scratchBucket
	// key is only the last returned key, independent of index size.
	key []byte
}

// First selects the first indexed key, including an empty key if one exists.
func (cursor *scratchCursor) First() ([]byte, []byte) {
	return cursor.seek(nil, false)
}

// Seek selects the first indexed key at or beyond key.
func (cursor *scratchCursor) Seek(key []byte) ([]byte, []byte) {
	return cursor.seek(key, false)
}

// Next selects the first indexed key strictly beyond the last returned key.
func (cursor *scratchCursor) Next() ([]byte, []byte) {
	return cursor.seek(cursor.key, true)
}

// seek copies one indexed row and closes its native iterator immediately.
func (cursor *scratchCursor) seek(key []byte, strict bool) ([]byte, []byte) {
	// Use the existing SQLite iterator's LIMIT 1 keyset reads, never a collected scan.
	bucket := cursor.bucket
	if bucket.tx.err != nil {
		return nil, nil
	}
	iterator := bucket.tx.tx.Iterate(bucket.tx.ctx, bucket.prefix, true, false)
	defer iterator.Close()

	// Seek by key without keeping a SQL row open across callback work. Keys
	// compare as bytes, so key||0x00 is the least key strictly beyond key, and
	// one LIMIT 1 read returns the next row without rereading the current one.
	full := append(bytes.Clone(bucket.prefix), key...)
	if strict {
		full = append(full, 0)
	}
	err := iterator.Seek(full)
	if err == nil {
		err = iterator.Err()
	}
	if err != nil {
		bucket.tx.fail(err)
		return nil, nil
	}
	if !iterator.Valid() {
		return nil, nil
	}

	// Copy the single row before releasing its native iterator.
	value, err := iterator.Value()
	if err != nil {
		bucket.tx.fail(err)
		return nil, nil
	}
	cursor.key = bytes.Clone(iterator.Key()[len(bucket.prefix):])
	return bytes.Clone(cursor.key), bytes.Clone(value)
}
