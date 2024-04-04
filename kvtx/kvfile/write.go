package kvtx_kvfile

import (
	"bytes"
	"context"
	"io"

	kvfile "github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/kvtx"
)

// FilterKeysFunc returns if the given key should be included in the file or not.
// Returning any error interrupts the entire process.
type FilterKeysFunc func(key []byte) (bool, error)

// KvfileFromStore builds a kvfile from a kvtx store.
//
// Note: does not support BlockIterator.
// filterKeys can be nil
func KvfileFromStore(ctx context.Context, writer io.Writer, store kvtx.Store, filterKeys FilterKeysFunc) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	return KvfileFromTx(ctx, writer, tx, filterKeys)
}

// KvfileFromTx builds a kvfile from a kvtx transaction.
//
// Note: does not support BlockIterator.
// filterKeys can be nil
func KvfileFromTx(ctx context.Context, writer io.Writer, tx kvtx.Tx, filterKeys FilterKeysFunc) error {
	it := tx.Iterate(ctx, nil, false, false)
	defer it.Close()

	return KvfileFromIterator(ctx, writer, it, filterKeys)
}

// KvfileFromIterator builds a kvfile from a kvtx iterator.
//
// Note: calls it.Next before the first key.
// Note: does not support BlockIterator.
// Note: does not close the iterator.
func KvfileFromIterator(ctx context.Context, writer io.Writer, it kvtx.Iterator, filterKeys FilterKeysFunc) error {
	buf := make([]byte, 2*1024)
	return kvfile.WriteIterator(
		writer,
		func() ([]byte, error) {
			// iterate until we have a non-skipped key or the stream ends
			for {
				if err := ctx.Err(); err != nil {
					return nil, context.Canceled
				}
				if !it.Next() {
					return nil, it.Err()
				}
				key := it.Key()
				if filterKeys != nil {
					ok, err := filterKeys(key)
					if err != nil {
						return nil, err
					}
					if !ok {
						continue
					}
				}
				return key, nil
			}
		},
		func(wr io.Writer, key []byte) (uint64, error) {
			if err := ctx.Err(); err != nil {
				return 0, context.Canceled
			}
			val, err := it.Value()
			if err != nil {
				return 0, err
			}
			nw, err := io.CopyBuffer(wr, bytes.NewReader(val), buf)
			if nw < 0 {
				return 0, err
			}
			return uint64(nw), err
		},
	)
}
