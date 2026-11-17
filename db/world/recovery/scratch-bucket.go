//go:build !js && !wasip1

package world_recovery

import (
	"bytes"
)

// scratchBucket is one named ordered index over the existing SQLite KV table.
type scratchBucket struct {
	// tx owns snapshot lifetime and error propagation.
	tx *scratchTx
	// prefix separates this index from every other index.
	prefix []byte
}

// Get reads one value, preserving failure separately from absent data.
func (bucket *scratchBucket) Get(key []byte) []byte {
	// Copy one row out of SQLite's transaction-owned buffers.
	if bucket.tx.err != nil {
		return nil
	}
	data, found, err := bucket.tx.tx.Get(bucket.tx.ctx, append(bytes.Clone(bucket.prefix), key...))
	bucket.tx.fail(err)
	if err != nil || !found {
		return nil
	}
	if data == nil {
		return []byte{}
	}
	return bytes.Clone(data)
}

// Put replaces one bounded index row within the enclosing atomic transaction.
func (bucket *scratchBucket) Put(key, value []byte) error {
	// Normalize empty values while retaining any prior transaction error.
	if bucket.tx.err != nil {
		return bucket.tx.err
	}
	if value == nil {
		value = []byte{}
	}
	err := bucket.tx.tx.Set(bucket.tx.ctx, append(bytes.Clone(bucket.prefix), key...), value)
	bucket.tx.fail(err)
	bucket.tx.written += len(bucket.prefix) + len(key) + len(value)
	return err
}

// Delete removes one index row within the enclosing atomic transaction.
func (bucket *scratchBucket) Delete(key []byte) error {
	if bucket.tx.err != nil {
		return bucket.tx.err
	}
	err := bucket.tx.tx.Delete(bucket.tx.ctx, append(bytes.Clone(bucket.prefix), key...))
	bucket.tx.fail(err)
	return err
}

// Cursor returns a keyset cursor that retains one bounded row at a time.
func (bucket *scratchBucket) Cursor() *scratchCursor {
	return &scratchCursor{bucket: bucket}
}
