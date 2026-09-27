//go:build js

package store_kvtx_indexeddb

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/util/ulid"
)

// TestIteratorObjectStore exercises empty, bounded, and resumed primary-key
// cursors in both directions against the browser's IndexedDB implementation.
func TestIteratorObjectStore(t *testing.T) {
	// Isolate the database and remove it after closing every transaction.
	ctx := context.Background()
	name := "spacewave-iterator-test-" + ulid.NewULID()
	store, err := Open(ctx, name, "keys")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Release the connection before requesting database deletion.
		if err := store.Close(); err != nil {
			t.Error(err)
		}

		// Wait for deletion so no test database survives this case.
		req, err := idb.Global().DeleteDatabase(name)
		if err != nil {
			t.Error(err)
			return
		}
		if err := req.Await(ctx); err != nil {
			t.Error(err)
		}
	})

	// An empty object store must terminate cleanly in either direction.
	for _, reverse := range []bool{false, true} {
		tx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		it := tx.Iterate(ctx, nil, true, reverse)
		valid := it.Next()
		err = it.Err()
		it.Close()
		tx.Discard()
		if err != nil {
			t.Fatalf("empty reverse=%v: %v", reverse, err)
		}
		if valid {
			t.Fatalf("empty reverse=%v returned a key", reverse)
		}
	}

	// Persist binary primary keys with identical values to verify key ordering.
	keys := [][]byte{{0x10}, {0x20}, {0x20, 0}, {0x20, 0xff}, {0x21}, {0xff}, {0xff, 0}}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	for _, key := range keys {
		if err := tx.Set(ctx, key, []byte("value")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Include the store tally when scanning every primary key.
	all := [][]byte{keys[0], keys[1], keys[2], keys[3], keys[4], tallyKey, keys[5], keys[6]}

	// Seek and continuation must preserve prefix bounds in both directions.
	for _, tc := range []struct {
		name    string
		prefix  []byte
		seek    []byte
		reverse bool
		want    [][]byte
	}{
		{name: "forward", want: all},
		{name: "reverse", reverse: true, want: [][]byte{keys[6], keys[5], tallyKey, keys[4], keys[3], keys[2], keys[1], keys[0]}},
		{name: "prefix-forward", prefix: []byte{0x20}, want: keys[1:4]},
		{name: "prefix-reverse", prefix: []byte{0x20}, reverse: true, want: [][]byte{keys[3], keys[2], keys[1]}},
		{name: "seek-forward", seek: keys[3], want: all[3:]},
		{name: "seek-reverse", seek: keys[2], reverse: true, want: [][]byte{keys[2], keys[1], keys[0]}},
		{name: "prefix-seek-forward", prefix: []byte{0x20}, seek: keys[2], want: keys[2:4]},
		{name: "prefix-seek-reverse", prefix: []byte{0x20}, seek: keys[2], reverse: true, want: [][]byte{keys[2], keys[1]}},
		{name: "unbounded-prefix-reverse", prefix: []byte{0xff}, reverse: true, want: [][]byte{keys[6], keys[5]}},
		{name: "missing-prefix", prefix: []byte{0x30}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Retain the read transaction until its iterator is closed.
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tx.Discard)
			it := tx.Iterate(ctx, tc.prefix, true, tc.reverse)
			t.Cleanup(it.Close)

			// Seek positions the cursor at the first matching primary key.
			if err := it.Seek(tc.seek); err != nil {
				t.Fatal(err)
			}
			var got [][]byte
			for it.Valid() {
				got = append(got, bytes.Clone(it.Key()))
				it.Next()
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}

			// Compare every key so skips, duplicates, and range escapes fail.
			if len(got) != len(tc.want) {
				t.Fatalf("key count: got %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if !bytes.Equal(got[i], tc.want[i]) {
					t.Fatalf("key %d: got %x, want %x", i, got[i], tc.want[i])
				}
			}
		})
	}
}
