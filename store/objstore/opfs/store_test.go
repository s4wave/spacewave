//go:build js

package store_objstore_opfs

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/hydra/opfs"
)

func TestObjectStoreReadWrite(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-objstore-basic", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-objstore-basic", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-objstore-basic|obj", "test-objstore-basic/obj")

	// Write a key.
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("key1"), []byte("value1")); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Read it back.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	val, found, err := rtx.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("key1 not found")
	}
	if !bytes.Equal(val, []byte("value1")) {
		t.Fatalf("got %q, want %q", val, "value1")
	}
}

func TestObjectStoreConcurrentReads(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-objstore-conc", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-objstore-conc", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-objstore-conc|obj", "test-objstore-conc/obj")

	// Seed some data.
	const n = 5
	for i := range n {
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		key := []byte("key-" + strconv.Itoa(i))
		val := []byte("val-" + strconv.Itoa(i))
		if err := tx.Set(ctx, key, val); err != nil {
			tx.Discard()
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Concurrent reads should all succeed.
	const readers = 10
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			rtx, err := s.NewTransaction(ctx, false)
			if err != nil {
				t.Error(err)
				return
			}
			defer rtx.Discard()

			for i := range n {
				key := []byte("key-" + strconv.Itoa(i))
				want := []byte("val-" + strconv.Itoa(i))
				val, found, err := rtx.Get(ctx, key)
				if err != nil {
					t.Error(err)
					return
				}
				if !found {
					t.Errorf("key-%d not found", i)
					return
				}
				if !bytes.Equal(val, want) {
					t.Errorf("key-%d: got %q, want %q", i, val, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestObjectStoreDelete(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, "test-objstore-del", true)
	if err != nil {
		t.Fatal(err)
	}
	defer opfs.DeleteEntry(root, "test-objstore-del", true) //nolint

	ctx := context.Background()
	s := NewStore(dir, "test-objstore-del|obj", "test-objstore-del/obj")

	// Write then delete.
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("gone"), []byte("data")); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tx2, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.Delete(ctx, []byte("gone")); err != nil {
		tx2.Discard()
		t.Fatal(err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify deleted.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	_, found, err := rtx.Get(ctx, []byte("gone"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("key 'gone' still found after delete")
	}
}
