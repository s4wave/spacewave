//go:build js

package store

import (
	"bytes"
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	opfs "github.com/s4wave/spacewave/prototypes/opfs/go-opfs"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	name := "test-store-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	root, err := opfs.GetRootDirectory()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := root.GetDirectoryHandle(name, true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, func() {
		s.Close()
		_ = root.RemoveEntry(name, true)
	}
}

func TestOPFSStoreOpen(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	tx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	_, found, err := tx.Get(ctx, []byte("nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected not found for nonexistent key")
	}
}

func TestOPFSStoreSetGet(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Write.
	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("hello")
	val := []byte("world")
	if err := wtx.Set(ctx, key, val); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Read back.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	got, found, err := rtx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("key not found after set")
	}
	if !bytes.Equal(got, val) {
		t.Fatalf("got %q, want %q", got, val)
	}
}

func TestOPFSStoreDelete(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Set a key.
	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("deleteme")
	if err := wtx.Set(ctx, key, []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Delete it.
	dtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := dtx.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := dtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Confirm gone.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	_, found, err := rtx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("key found after delete")
	}
}

func TestOPFSStoreExistsSize(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	keys := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	for _, k := range keys {
		if err := wtx.Set(ctx, k, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	for _, k := range keys {
		exists, err := rtx.Exists(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("key %q should exist", k)
		}
	}

	exists, err := rtx.Exists(ctx, []byte("missing"))
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing key should not exist")
	}

	size, err := rtx.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 3 {
		t.Fatalf("Size: got %d, want 3", size)
	}
}

func TestHandleCacheRefcount(t *testing.T) {
	root, err := opfs.GetRootDirectory()
	if err != nil {
		t.Fatal(err)
	}
	name := "test-cache-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dir, err := root.GetDirectoryHandle(name, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.RemoveEntry(name, true) }()

	fh, err := dir.GetFileHandle("test.bin", true)
	if err != nil {
		t.Fatal(err)
	}

	cache := newHandleCache()

	// Acquire twice.
	h1, err := cache.acquire("test.bin", fh)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.acquire("test.bin", fh)
	if err != nil {
		t.Fatal(err)
	}

	// Write through h1.
	_, err = h1.Write([]byte("data"))
	if err != nil {
		t.Fatal(err)
	}

	// Release once, handle should stay open.
	cache.release("test.bin")
	cache.mu.Lock()
	entry := cache.entries["test.bin"]
	cache.mu.Unlock()
	if entry == nil || entry.refcount != 1 {
		t.Fatal("expected refcount 1 after one release")
	}

	// Release again, handle should close.
	cache.release("test.bin")
	cache.mu.Lock()
	_, exists := cache.entries["test.bin"]
	cache.mu.Unlock()
	if exists {
		t.Fatal("expected entry removed after all releases")
	}

	// Re-acquire should open a fresh handle.
	_, err = cache.acquire("test.bin", fh)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.release("test.bin")
}

func TestOPFSStoreIterate(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Write keys with shared prefix.
	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	prefix := []byte("prefix/")
	keys := [][]byte{
		append([]byte(nil), append(prefix, []byte("alpha")...)...),
		append([]byte(nil), append(prefix, []byte("beta")...)...),
		append([]byte(nil), append(prefix, []byte("gamma")...)...),
	}
	for _, k := range keys {
		if err := wtx.Set(ctx, k, k); err != nil {
			t.Fatal(err)
		}
	}
	// Also write a key without the prefix.
	if err := wtx.Set(ctx, []byte("other"), []byte("val")); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Iterate with prefix.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	it := rtx.Iterate(ctx, prefix, true, false)
	var got [][]byte
	for it.Next() {
		k := make([]byte, len(it.Key()))
		copy(k, it.Key())
		got = append(got, k)
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	it.Close()

	if len(got) != len(keys) {
		t.Fatalf("iterate: got %d keys, want %d", len(got), len(keys))
	}

	// Should be sorted.
	sorted := make([][]byte, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i], sorted[j]) < 0
	})
	for i, k := range got {
		if !bytes.Equal(k, sorted[i]) {
			t.Fatalf("iterate[%d]: got %q, want %q", i, k, sorted[i])
		}
	}
}

func TestOPFSStoreScanPrefix(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	prefix := []byte("scan/")
	keys := [][]byte{
		append([]byte(nil), append(prefix, []byte("one")...)...),
		append([]byte(nil), append(prefix, []byte("two")...)...),
		append([]byte(nil), append(prefix, []byte("three")...)...),
	}
	for _, k := range keys {
		if err := wtx.Set(ctx, k, k); err != nil {
			t.Fatal(err)
		}
	}
	if err := wtx.Set(ctx, []byte("noscan"), []byte("val")); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	var found [][]byte
	err = rtx.ScanPrefix(ctx, prefix, func(key, val []byte) error {
		k := make([]byte, len(key))
		copy(k, key)
		found = append(found, k)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(keys) {
		t.Fatalf("ScanPrefix: got %d keys, want %d", len(found), len(keys))
	}
}

func TestOPFSStoreTally(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	val := []byte("12345") // 5 bytes each
	if err := wtx.Set(ctx, []byte("k1"), val); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Set(ctx, []byte("k2"), val); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tally := s.GetStorageTally()
	if tally != 10 {
		t.Fatalf("tally: got %d, want 10", tally)
	}

	// Save and reload.
	if err := s.saveTally(); err != nil {
		t.Fatal(err)
	}

	// Create new store from same directory to test tally load.
	s2, err := NewStore(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if err := initTally(s2); err != nil {
		t.Fatal(err)
	}
	tally2 := s2.GetStorageTally()
	if tally2 != 10 {
		t.Fatalf("tally after reload: got %d, want 10", tally2)
	}
}

func TestOPFSStoreTxDiscard(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Write in a transaction, then discard.
	wtx, err := s.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := wtx.Set(ctx, []byte("discarded"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	wtx.Discard()

	// Confirm key was not persisted.
	rtx, err := s.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()

	_, found, err := rtx.Get(ctx, []byte("discarded"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("discarded key should not be found")
	}
}
