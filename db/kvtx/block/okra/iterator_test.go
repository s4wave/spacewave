package kvtx_block_okra

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block/blob"
)

func TestTxIteratorSeekPrefixAndValueCursor(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()
	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"aa/0", "aa/1", "aa/2", "ab/0", "b/0"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	forward := readTx.Iterate(ctx, []byte("aa/"), true, false)
	defer forward.Close()
	if err := forward.Seek(nil); err != nil {
		t.Fatal(err)
	}
	assertOkraIteratorKeys(t, forward, []string{"aa/0", "aa/1", "aa/2"})

	reverse := readTx.Iterate(ctx, []byte("aa/"), true, true)
	defer reverse.Close()
	if err := reverse.Seek(nil); err != nil {
		t.Fatal(err)
	}
	assertOkraIteratorKeys(t, reverse, []string{"aa/2", "aa/1", "aa/0"})

	seekForward := readTx.Iterate(ctx, []byte("aa/"), true, false)
	defer seekForward.Close()
	if err := seekForward.Seek([]byte("aa/1.5")); err != nil {
		t.Fatal(err)
	}
	assertOkraIteratorKeys(t, seekForward, []string{"aa/2"})

	seekReverse := readTx.Iterate(ctx, []byte("aa/"), true, true)
	defer seekReverse.Close()
	if err := seekReverse.Seek([]byte("aa/1.5")); err != nil {
		t.Fatal(err)
	}
	assertOkraIteratorKeys(t, seekReverse, []string{"aa/1", "aa/0"})

	missing := readTx.Iterate(ctx, []byte("missing/"), true, false)
	defer missing.Close()
	if err := missing.Seek(nil); err != nil {
		t.Fatal(err)
	}
	if missing.Valid() {
		t.Fatalf("missing prefix iterator valid at %q", missing.Key())
	}
	if err := missing.Err(); err != nil {
		t.Fatal(err)
	}

	blockIter := readTx.BlockIterate(ctx, []byte("aa/"), true, false)
	defer blockIter.Close()
	if err := blockIter.Seek(nil); err != nil {
		t.Fatal(err)
	}
	if !blockIter.Valid() {
		t.Fatal("expected valid block iterator")
	}
	valueCursor := blockIter.ValueCursor()
	if valueCursor == nil {
		t.Fatal("missing value cursor")
	}
	value, err := blob.FetchToBytes(ctx, valueCursor)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value, []byte("value-aa/0")) {
		t.Fatalf("value cursor = %q, want value-aa/0", value)
	}
}

func TestTxScanPrefix(t *testing.T) {
	ctx := context.Background()
	store := newOkraTestStore()
	rootRef := writeMutatedOkraRoot(t, ctx, store, func(tx *Tx) {
		for _, key := range []string{"aa/0", "aa/1", "ab/0", "b/0"} {
			if err := tx.Set(ctx, []byte(key), []byte("value-"+key)); err != nil {
				t.Fatal(err)
			}
		}
	})

	readTx := openOkraRoot(t, ctx, store, rootRef, false)
	defer readTx.Discard()

	var keys []string
	err := readTx.ScanPrefix(ctx, []byte("aa/"), func(key, value []byte) error {
		keys = append(keys, string(key)+"="+string(value))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aa/0=value-aa/0", "aa/1=value-aa/1"}; !equalStrings(keys, want) {
		t.Fatalf("ScanPrefix = %v, want %v", keys, want)
	}

	keys = nil
	err = readTx.ScanPrefixKeys(ctx, []byte("aa/"), func(key []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aa/0", "aa/1"}; !equalStrings(keys, want) {
		t.Fatalf("ScanPrefixKeys = %v, want %v", keys, want)
	}
}

func assertOkraIteratorKeys(t *testing.T, iter interface {
	Valid() bool
	Key() []byte
	Next() bool
	Err() error
}, expected []string) {
	t.Helper()
	for idx, exp := range expected {
		if idx != 0 && !iter.Next() {
			t.Fatalf("iterator stopped before %s", exp)
		}
		if !iter.Valid() {
			t.Fatalf("iterator invalid, expected key %s", exp)
		}
		if got := string(iter.Key()); got != exp {
			t.Fatalf("iterator key = %s, want %s", got, exp)
		}
	}
	if iter.Next() || iter.Valid() {
		t.Fatalf("iterator still valid at %q", iter.Key())
	}
	if iter.Next() || iter.Valid() {
		t.Fatalf("iterator restarted after exhaustion at %q", iter.Key())
	}
	if err := iter.Err(); err != nil {
		t.Fatal(err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}
