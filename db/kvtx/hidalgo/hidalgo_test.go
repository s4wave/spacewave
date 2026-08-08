package kvtx_hidalgo

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/cayley/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/kv/kvtest"
	"github.com/aperturerobotics/cayley/kv/options"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestKVTX(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	kvtest.RunTestLocal(t, func(path string) (kv.KV, error) {
		return flat.Upgrade(NewKV(store_kvtx_inmem.NewStore())), nil
	}, nil)
}

func TestTxScanUsesLazyIterator(t *testing.T) {
	ctx := context.Background()
	prefix := kv.Key{[]byte("quad"), []byte("subject")}
	flatPrefix := flat.KeyEscape(prefix)
	spy := &lazyScanTx{
		entries: []lazyScanEntry{
			{key: append(bytes.Clone(flatPrefix), byte(0x01)), value: []byte("one")},
			{key: append(bytes.Clone(flatPrefix), byte(0x02)), value: []byte("two")},
			{key: append(bytes.Clone(flatPrefix), byte(0x03)), value: []byte("three")},
		},
	}

	it := NewTx(spy).Scan(ctx, options.WithPrefixKV(prefix))
	if spy.scanPrefixCalls != 0 {
		t.Fatalf("ScanPrefix called during Scan: %d", spy.scanPrefixCalls)
	}
	if spy.iterateCalls != 0 {
		t.Fatalf("Iterate called before first Next: %d", spy.iterateCalls)
	}

	if !it.Next(ctx) {
		t.Fatalf("expected first item, err=%v", it.Err())
	}
	if spy.scanPrefixCalls != 0 {
		t.Fatalf("ScanPrefix called after first Next: %d", spy.scanPrefixCalls)
	}
	if spy.iterateCalls != 1 {
		t.Fatalf("Iterate calls = %d, want 1", spy.iterateCalls)
	}
	if !bytes.Equal(spy.iteratePrefix, flatPrefix) {
		t.Fatalf("Iterate prefix = %x, want %x", spy.iteratePrefix, flatPrefix)
	}
	if !spy.iterateSort || spy.iterateReverse {
		t.Fatalf("Iterate sort/reverse = %v/%v, want true/false", spy.iterateSort, spy.iterateReverse)
	}
	if spy.lastIterator.seekCalls != 1 {
		t.Fatalf("Seek calls = %d, want 1", spy.lastIterator.seekCalls)
	}
	if spy.lastIterator.nextCalls != 0 {
		t.Fatalf("Next calls = %d, want 0 before second item", spy.lastIterator.nextCalls)
	}
	if spy.lastIterator.valueCopyCalls != 1 {
		t.Fatalf("ValueCopy calls = %d, want 1", spy.lastIterator.valueCopyCalls)
	}
	if got := it.Val(); !bytes.Equal(got, []byte("one")) {
		t.Fatalf("Val = %q, want one", got)
	}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
	if !spy.lastIterator.closed {
		t.Fatal("underlying iterator was not closed")
	}
}

type lazyScanEntry struct {
	key   []byte
	value []byte
}

type lazyScanTx struct {
	entries []lazyScanEntry

	scanPrefixCalls int
	iterateCalls    int
	iteratePrefix   []byte
	iterateSort     bool
	iterateReverse  bool
	lastIterator    *lazyScanIterator
}

func (t *lazyScanTx) Size(ctx context.Context) (uint64, error) {
	return uint64(len(t.entries)), nil
}

func (t *lazyScanTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	for _, entry := range t.entries {
		if bytes.Equal(entry.key, key) {
			return bytes.Clone(entry.value), true, nil
		}
	}
	return nil, false, nil
}

func (t *lazyScanTx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.Get(ctx, key)
	return found, err
}

func (t *lazyScanTx) Set(ctx context.Context, key, value []byte) error {
	return kvtx.ErrNotWrite
}

func (t *lazyScanTx) Delete(ctx context.Context, key []byte) error {
	return kvtx.ErrNotWrite
}

func (t *lazyScanTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	t.scanPrefixCalls++
	return nil
}

func (t *lazyScanTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

func (t *lazyScanTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	t.iterateCalls++
	t.iteratePrefix = bytes.Clone(prefix)
	t.iterateSort = sort
	t.iterateReverse = reverse
	t.lastIterator = &lazyScanIterator{
		prefix:  bytes.Clone(prefix),
		entries: t.entries,
		pos:     -1,
	}
	return t.lastIterator
}

func (t *lazyScanTx) Commit(ctx context.Context) error {
	return kvtx.ErrNotWrite
}

func (t *lazyScanTx) Discard() {}

type lazyScanIterator struct {
	prefix  []byte
	entries []lazyScanEntry
	pos     int
	err     error

	seekCalls      int
	nextCalls      int
	valueCopyCalls int
	closed         bool
}

func (i *lazyScanIterator) Err() error {
	return i.err
}

func (i *lazyScanIterator) Valid() bool {
	return i.err == nil && i.pos >= 0 && i.pos < len(i.entries)
}

func (i *lazyScanIterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.entries[i.pos].key
}

func (i *lazyScanIterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.err
	}
	return i.entries[i.pos].value, nil
}

func (i *lazyScanIterator) ValueCopy(dst []byte) ([]byte, error) {
	i.valueCopyCalls++
	value, err := i.Value()
	if err != nil || value == nil {
		return nil, err
	}
	return append(dst[:0], value...), nil
}

func (i *lazyScanIterator) Next() bool {
	i.nextCalls++
	if i.err != nil {
		return false
	}
	i.pos++
	return i.Valid()
}

func (i *lazyScanIterator) Seek(k []byte) error {
	i.seekCalls++
	if i.err != nil {
		return i.err
	}
	if len(k) != 0 {
		for idx, entry := range i.entries {
			if bytes.Compare(entry.key, k) >= 0 {
				i.pos = idx
				return nil
			}
		}
		i.pos = -1
		return nil
	}
	for idx, entry := range i.entries {
		if len(i.prefix) == 0 || bytes.HasPrefix(entry.key, i.prefix) {
			i.pos = idx
			return nil
		}
	}
	i.pos = -1
	return nil
}

func (i *lazyScanIterator) Close() {
	i.closed = true
	i.pos = -1
}

// _ is a type assertion
var (
	_ kvtx.Tx       = (*lazyScanTx)(nil)
	_ kvtx.Iterator = (*lazyScanIterator)(nil)
)
