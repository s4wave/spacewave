package store_kvtx_badger

import (
	"context"
	"slices"
	"testing"

	bdb "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/kvtx"
)

// newIterTestTx opens an in-memory store holding keys and returns a read tx.
func newIterTestTx(t *testing.T, keys ...string) kvtx.Tx {
	// Open an in-memory Badger store for iterator traversal tests.
	ctx := context.Background()
	db, err := Open(bdb.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.db.Close() })

	// Populate and commit the keys that the iterator will traverse.
	wtx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if err := wtx.Set(ctx, []byte(k), []byte("v:"+k)); err != nil {
			t.Fatal(err)
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Return a read transaction on the populated Badger snapshot.
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rtx.Discard)
	return rtx
}

// collectIter reads the current entry and all following entries.
func collectIter(it kvtx.Iterator) []string {
	var out []string
	for ok := it.Valid(); ok; ok = it.Next() {
		out = append(out, string(it.Key()))
	}
	return out
}

func TestIteratorNextBeforeSeek(t *testing.T) {
	ctx := context.Background()
	tx := newIterTestTx(t, "a/1", "a/2", "b/1")
	for _, rev := range []bool{false, true} {
		it := tx.Iterate(ctx, []byte("a/"), true, rev)
		if !it.Next() {
			t.Fatalf("rev=%v: Next before Seek returned false", rev)
		}
		got := collectIter(it)
		want := []string{"a/1", "a/2"}
		if rev {
			slices.Reverse(want)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("rev=%v: got %v, want %v", rev, got, want)
		}
		if it.Next() {
			t.Fatalf("rev=%v: Next after end returned true", rev)
		}
		it.Close()
	}
}

func TestIteratorReverseSeekPrefixBounds(t *testing.T) {
	// Create a reverse Badger iterator over the test prefix.
	ctx := context.Background()

	// "b" is the prefix successor of "a".
	tx := newIterTestTx(t, "0", "a", "a/1", "a/2", "b", "b/1")
	it := tx.Iterate(ctx, []byte("a"), true, true)
	defer it.Close()

	// Check reverse traversal from the end of the Badger prefix.
	if err := it.Seek(nil); err != nil {
		t.Fatal(err)
	}
	if got, want := collectIter(it), []string{"a/2", "a/1", "a"}; !slices.Equal(got, want) {
		t.Fatalf("Seek(nil): got %v, want %v", got, want)
	}

	// Check reverse traversal when the seek key lies beyond the prefix.
	if err := it.Seek([]byte("c")); err != nil {
		t.Fatal(err)
	}
	if got, want := collectIter(it), []string{"a/2", "a/1", "a"}; !slices.Equal(got, want) {
		t.Fatalf("Seek(past prefix): got %v, want %v", got, want)
	}

	// Check reverse traversal from a key inside the Badger prefix.
	if err := it.Seek([]byte("a/1")); err != nil {
		t.Fatal(err)
	}
	if got, want := collectIter(it), []string{"a/1", "a"}; !slices.Equal(got, want) {
		t.Fatalf("Seek(a/1): got %v, want %v", got, want)
	}

	// Check that a reverse seek before the Badger prefix is invalid.
	if err := it.Seek([]byte("0")); err != nil {
		t.Fatal(err)
	}
	if it.Valid() {
		t.Fatalf("Seek(before prefix): valid at %q", it.Key())
	}
}
