package kvtx_block

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

func TestKVTXBackendGetBatchMatchesScalarGet(t *testing.T) {
	ctx := context.Background()
	for _, impl := range backendConformanceImpls() {
		t.Run(impl.String(), func(t *testing.T) {
			root := newBackendConformanceRoot(t, ctx, impl)
			_, btx, tx := root.newWriteTx(t, ctx)
			for _, entry := range []struct {
				key   string
				value string
			}{
				{key: "alpha", value: "one"},
				{key: "bravo", value: "two"},
				{key: "charlie", value: "three"},
			} {
				if err := tx.Set(ctx, []byte(entry.key), []byte(entry.value)); err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
			root.commit(t, ctx, btx, tx)

			readTx := root.newReadTx(t, ctx)
			defer readTx.Discard()
			batchTx, ok := readTx.(kvtx.BatchTxOps)
			if !ok {
				t.Fatalf("%s read transaction does not implement kvtx.BatchTxOps", impl)
			}
			keys := [][]byte{
				[]byte("bravo"),
				[]byte("missing"),
				[]byte("alpha"),
				[]byte("bravo"),
				[]byte("charlie"),
			}

			batchValues, batchFound, err := batchTx.GetBatch(ctx, keys)
			if err != nil {
				t.Fatal(err)
			}
			if len(batchValues) != len(keys) {
				t.Fatalf("values len = %d, want %d", len(batchValues), len(keys))
			}
			if len(batchFound) != len(keys) {
				t.Fatalf("found len = %d, want %d", len(batchFound), len(keys))
			}
			for i, key := range keys {
				scalarValue, scalarFound, err := readTx.Get(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				if batchFound[i] != scalarFound || !bytes.Equal(batchValues[i], scalarValue) {
					t.Fatalf("GetBatch(%q)[%d] = %q, %v, scalar Get = %q, %v", key, i, batchValues[i], batchFound[i], scalarValue, scalarFound)
				}
			}
		})
	}
}

func TestKVTXBackendGetBatchRejectsEmptyKey(t *testing.T) {
	ctx := context.Background()
	for _, impl := range backendConformanceImpls() {
		t.Run(impl.String(), func(t *testing.T) {
			root := newBackendConformanceRoot(t, ctx, impl)
			readTx := root.newReadTx(t, ctx)
			defer readTx.Discard()
			batchTx, ok := readTx.(kvtx.BatchTxOps)
			if !ok {
				t.Fatalf("%s read transaction does not implement kvtx.BatchTxOps", impl)
			}

			_, _, err := batchTx.GetBatch(ctx, [][]byte{[]byte("present"), nil})
			if !errors.Is(err, kvtx.ErrEmptyKey) {
				t.Fatalf("GetBatch empty key err = %v, want %v", err, kvtx.ErrEmptyKey)
			}
		})
	}
}
