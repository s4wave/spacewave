package kvtx_hidalgo

import (
	"bytes"
	"context"
	"testing"

	flat "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/s4wave/spacewave/db/kvtx"
)

func TestTxGetBatchUsesUnderlyingBatchAndAlignsResults(t *testing.T) {
	ctx := context.Background()
	lower := &batchGetSpyTx{
		values: map[string][]byte{
			"alpha": []byte("one"),
			"bravo": []byte("two"),
		},
	}

	values, err := NewTx(lower).GetBatch(ctx, []flat.Key{
		flat.Key("bravo"),
		nil,
		flat.Key("missing"),
		flat.Key("alpha"),
		{},
		flat.Key("bravo"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if lower.batchCalls != 1 {
		t.Fatalf("underlying GetBatch calls = %d, want 1", lower.batchCalls)
	}
	if lower.scalarGets != 0 {
		t.Fatalf("scalar Get calls = %d, want 0", lower.scalarGets)
	}
	assertByteKeys(t, "underlying batch keys", lower.batchKeys, [][]byte{
		[]byte("bravo"),
		[]byte("missing"),
		[]byte("alpha"),
		[]byte("bravo"),
	})
	assertFlatValues(t, values, []flat.Value{
		flat.Value("two"),
		nil,
		nil,
		flat.Value("one"),
		nil,
		flat.Value("two"),
	})
}

type batchGetSpyTx struct {
	values map[string][]byte

	batchCalls int
	batchKeys  [][]byte
	scalarGets int
}

func (t *batchGetSpyTx) Size(context.Context) (uint64, error) {
	return uint64(len(t.values)), nil
}

func (t *batchGetSpyTx) Get(_ context.Context, key []byte) ([]byte, bool, error) {
	t.scalarGets++
	value, ok := t.values[string(key)]
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(value), true, nil
}

func (t *batchGetSpyTx) GetBatch(_ context.Context, keys [][]byte) ([][]byte, []bool, error) {
	t.batchCalls++
	t.batchKeys = cloneByteSlices(keys)
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	for i, key := range keys {
		value, ok := t.values[string(key)]
		if ok {
			values[i] = bytes.Clone(value)
			found[i] = true
		}
	}
	return values, found, nil
}

func (t *batchGetSpyTx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.Get(ctx, key)
	return found, err
}

func (t *batchGetSpyTx) Set(context.Context, []byte, []byte) error {
	return kvtx.ErrNotWrite
}

func (t *batchGetSpyTx) Delete(context.Context, []byte) error {
	return kvtx.ErrNotWrite
}

func (t *batchGetSpyTx) ScanPrefix(context.Context, []byte, func([]byte, []byte) error) error {
	return nil
}

func (t *batchGetSpyTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func([]byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error { return cb(key) })
}

func (t *batchGetSpyTx) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return emptyBatchSpyIterator{}
}

func (t *batchGetSpyTx) Commit(context.Context) error {
	return kvtx.ErrNotWrite
}

func (t *batchGetSpyTx) Discard() {}

type emptyBatchSpyIterator struct{}

func (emptyBatchSpyIterator) Err() error                           { return nil }
func (emptyBatchSpyIterator) Valid() bool                          { return false }
func (emptyBatchSpyIterator) Key() []byte                          { return nil }
func (emptyBatchSpyIterator) Value() ([]byte, error)               { return nil, nil }
func (emptyBatchSpyIterator) ValueCopy(dst []byte) ([]byte, error) { return dst[:0], nil }
func (emptyBatchSpyIterator) Next() bool                           { return false }
func (emptyBatchSpyIterator) Seek([]byte) error                    { return nil }
func (emptyBatchSpyIterator) Close()                               {}

func assertByteKeys(t *testing.T, label string, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s len = %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

func assertFlatValues(t *testing.T, got, want []flat.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("values[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func cloneByteSlices(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, value := range in {
		out[i] = bytes.Clone(value)
	}
	return out
}
