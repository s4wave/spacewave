package kvtx_kvfile

import (
	"bytes"
	"context"
	"io"

	kvfile "github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/hydra/kvtx"
)

// KvfileFromStore builds a kvfile from a kvtx store.
//
// Note: does not support BlockIterator.
func KvfileFromStore(ctx context.Context, writer io.Writer, store kvtx.Store) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	return KvfileFromTx(ctx, writer, tx)
}

// KvfileFromTx builds a kvfile from a kvtx transaction.
//
// Note: does not support BlockIterator.
func KvfileFromTx(ctx context.Context, writer io.Writer, tx kvtx.Tx) error {
	it := tx.Iterate(ctx, nil, false, false)
	defer it.Close()

	return KvfileFromIterator(ctx, writer, it)
}

// KvfileFromIterator builds a kvfile from a kvtx iterator.
//
// Note: calls it.Next before the first key.
// Note: does not support BlockIterator.
// Note: does not close the iterator.
func KvfileFromIterator(ctx context.Context, writer io.Writer, it kvtx.Iterator) error {
	buf := make([]byte, 2*1024)
	return kvfile.WriteIterator(writer, func() ([]byte, error) {
		if !it.Next() {
			return nil, it.Err()
		}
		return it.Key(), nil
	}, func(wr io.Writer, key []byte) (uint64, error) {
		select {
		case <-ctx.Done():
			return 0, context.Canceled
		default:
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
	})
}
