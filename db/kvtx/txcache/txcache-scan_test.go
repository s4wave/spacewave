package kvtx_txcache

import (
	"context"
	"errors"
	"slices"
	"testing"

	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// newTestTXCache builds a TXCache over a store holding the given keys.
func newTestTXCache(t *testing.T, sortScan bool, keys ...string) *TXCache {
	ctx := context.Background()
	store := sinmem.NewStore()
	wtx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if err := wtx.Set(ctx, []byte(k), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rtx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rtx.Discard)
	return NewTXCache(rtx, sortScan)
}

// TestScanPrefixFiltersPendingSets verifies ScanPrefix only returns pending
// sets under the prefix.
func TestScanPrefixFiltersPendingSets(t *testing.T) {
	ctx := context.Background()
	for _, sortScan := range []bool{true, false} {
		tc := newTestTXCache(t, sortScan, "a/0", "b/0")
		for _, k := range []string{"a", "a/1", "b/1", "c"} {
			if err := tc.Set(ctx, []byte(k), []byte("v")); err != nil {
				t.Fatal(err)
			}
		}
		var got []string
		err := tc.ScanPrefix(ctx, []byte("a/"), func(key, value []byte) error {
			got = append(got, string(key))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		if want := []string{"a/0", "a/1"}; !slices.Equal(got, want) {
			t.Fatalf("sortScan=%v: got %v, want %v", sortScan, got, want)
		}
	}
}

// TestScanPrefixUnsortedReturnsCallbackError verifies a callback error on a
// pending set is returned.
func TestScanPrefixUnsortedReturnsCallbackError(t *testing.T) {
	ctx := context.Background()
	tc := newTestTXCache(t, false)
	if err := tc.Set(ctx, []byte("a/1"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	errStop := errors.New("stop")
	err := tc.ScanPrefix(ctx, []byte("a/"), func(key, value []byte) error {
		return errStop
	})
	if !errors.Is(err, errStop) {
		t.Fatalf("got %v, want %v", err, errStop)
	}
}

// TestDeleteCopiesKey verifies Delete does not retain the caller's key slice.
func TestDeleteCopiesKey(t *testing.T) {
	ctx := context.Background()
	tc := newTestTXCache(t, true, "abc")
	key := []byte("abc")
	if err := tc.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	copy(key, "xyz")
	exists, err := tc.Exists(ctx, []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("deleted key abc still exists after caller reused its slice")
	}
}

// TestSizeCountsPendingChanges verifies Size counts overwrites and deletes of
// missing keys correctly.
func TestSizeCountsPendingChanges(t *testing.T) {
	ctx := context.Background()
	tc := newTestTXCache(t, true, "a", "b")
	// overwrite, new key, delete existing, delete missing
	for _, k := range []string{"a", "c"} {
		if err := tc.Set(ctx, []byte(k), []byte("v2")); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"b", "missing"} {
		if err := tc.Delete(ctx, []byte(k)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := tc.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("size = %d, want 2", n)
	}

	empty := newTestTXCache(t, true)
	if err := empty.Delete(ctx, []byte("missing")); err != nil {
		t.Fatal(err)
	}
	n, err = empty.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("size = %d, want 0", n)
	}
}
