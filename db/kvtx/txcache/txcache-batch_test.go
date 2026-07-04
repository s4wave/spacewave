package kvtx_txcache

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

func TestTXCacheGetBatchAlignsCachedRemovedAndUnderlyingResults(t *testing.T) {
	ctx := context.Background()
	underlying := &txCacheBatchSpyOps{
		values: map[string][]byte{
			"underlying": []byte("from-underlying"),
			"removed":    []byte("should-not-leak"),
		},
	}
	cache := NewTXCache(underlying, false)
	if err := cache.Set(ctx, []byte("cached"), []byte("from-cache")); err != nil {
		t.Fatal(err)
	}
	if err := cache.Delete(ctx, []byte("removed")); err != nil {
		t.Fatal(err)
	}

	values, found, err := cache.GetBatch(ctx, [][]byte{
		[]byte("cached"),
		[]byte("removed"),
		[]byte("missing"),
		[]byte("underlying"),
		[]byte("cached"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if underlying.batchCalls != 1 {
		t.Fatalf("underlying GetBatch calls = %d, want 1", underlying.batchCalls)
	}
	if underlying.scalarGets != 0 {
		t.Fatalf("underlying scalar Get calls = %d, want 0", underlying.scalarGets)
	}
	assertTXCacheBatchKeys(t, underlying.batchKeys, [][]byte{
		[]byte("missing"),
		[]byte("underlying"),
	})
	assertTXCacheBatchResults(t, values, found, []txCacheBatchResult{
		{value: []byte("from-cache"), found: true},
		{found: false},
		{found: false},
		{value: []byte("from-underlying"), found: true},
		{value: []byte("from-cache"), found: true},
	})
}

func TestTXCacheGetBatchRejectsEmptyKeyBeforeUnderlyingLookup(t *testing.T) {
	ctx := context.Background()
	underlying := &txCacheBatchSpyOps{values: map[string][]byte{"present": []byte("value")}}
	cache := NewTXCache(underlying, false)

	_, _, err := cache.GetBatch(ctx, [][]byte{[]byte("present"), nil})
	if !errors.Is(err, kvtx.ErrEmptyKey) {
		t.Fatalf("GetBatch empty key err = %v, want %v", err, kvtx.ErrEmptyKey)
	}
	if underlying.batchCalls != 0 || underlying.scalarGets != 0 {
		t.Fatalf("underlying lookups after empty key: batch=%d scalar=%d, want 0/0", underlying.batchCalls, underlying.scalarGets)
	}
}

type txCacheBatchSpyOps struct {
	values map[string][]byte

	batchCalls int
	batchKeys  [][]byte
	scalarGets int
}

func (t *txCacheBatchSpyOps) Size(context.Context) (uint64, error) {
	return uint64(len(t.values)), nil
}

func (t *txCacheBatchSpyOps) Get(_ context.Context, key []byte) ([]byte, bool, error) {
	t.scalarGets++
	value, ok := t.values[string(key)]
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(value), true, nil
}

func (t *txCacheBatchSpyOps) GetBatch(_ context.Context, keys [][]byte) ([][]byte, []bool, error) {
	t.batchCalls++
	t.batchKeys = cloneTXCacheBatchBytes(keys)
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

func (t *txCacheBatchSpyOps) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.Get(ctx, key)
	return found, err
}

func (t *txCacheBatchSpyOps) Set(context.Context, []byte, []byte) error {
	return kvtx.ErrNotWrite
}

func (t *txCacheBatchSpyOps) Delete(context.Context, []byte) error {
	return kvtx.ErrNotWrite
}

func (t *txCacheBatchSpyOps) ScanPrefix(context.Context, []byte, func([]byte, []byte) error) error {
	return nil
}

func (t *txCacheBatchSpyOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func([]byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error { return cb(key) })
}

func (t *txCacheBatchSpyOps) Iterate(context.Context, []byte, bool, bool) kvtx.Iterator {
	return emptyTXCacheBatchIterator{}
}

type emptyTXCacheBatchIterator struct{}

func (emptyTXCacheBatchIterator) Err() error                           { return nil }
func (emptyTXCacheBatchIterator) Valid() bool                          { return false }
func (emptyTXCacheBatchIterator) Key() []byte                          { return nil }
func (emptyTXCacheBatchIterator) Value() ([]byte, error)               { return nil, nil }
func (emptyTXCacheBatchIterator) ValueCopy(dst []byte) ([]byte, error) { return dst[:0], nil }
func (emptyTXCacheBatchIterator) Next() bool                           { return false }
func (emptyTXCacheBatchIterator) Seek([]byte) error                    { return nil }
func (emptyTXCacheBatchIterator) Close()                               {}

type txCacheBatchResult struct {
	value []byte
	found bool
}

func assertTXCacheBatchResults(t *testing.T, values [][]byte, found []bool, want []txCacheBatchResult) {
	t.Helper()
	if len(values) != len(want) {
		t.Fatalf("values len = %d, want %d", len(values), len(want))
	}
	if len(found) != len(want) {
		t.Fatalf("found len = %d, want %d", len(found), len(want))
	}
	for i := range want {
		if found[i] != want[i].found || !bytes.Equal(values[i], want[i].value) {
			t.Fatalf("result[%d] = %q, %v, want %q, %v", i, values[i], found[i], want[i].value, want[i].found)
		}
	}
}

func assertTXCacheBatchKeys(t *testing.T, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("underlying batch key len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("underlying batch key[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func cloneTXCacheBatchBytes(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, value := range in {
		out[i] = bytes.Clone(value)
	}
	return out
}
