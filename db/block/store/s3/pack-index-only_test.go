package block_store_s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// requestCounter counts the reads and listings of an object store, and the
// listings alone.
type requestCounter struct {
	ObjectStore
	requests atomic.Int64
	lists    atomic.Int64
}

func (c *requestCounter) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	c.requests.Add(1)
	return c.ObjectStore.GetObject(ctx, bucket, key)
}

func (c *requestCounter) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	c.requests.Add(1)
	return c.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

func (c *requestCounter) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	c.requests.Add(1)
	c.lists.Add(1)
	return c.ObjectStore.ListObjects(ctx, bucket, prefix, fn)
}

// TestPackStoreIndexOnly checks that an index-only store asks the bucket for
// nothing its index does not place: a pack another writer left under the
// prefix stays unseen through a cold start, a miss and a warm restart, while
// the store's own packs read back.
func TestPackStoreIndexOnly(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	batch := func(name string, count int) ([]*block.PutBatchEntry, []*block.BlockRef) {
		var entries []*block.PutBatchEntry
		var refs []*block.BlockRef
		for i := range count {
			data := []byte(fmt.Sprintf("%s block %d", name, i))
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data})
			refs = append(refs, ref)
		}
		return entries, refs
	}

	// Another writer leaves a pack under the prefix.
	foreign, foreignRefs := batch("foreign", 10)
	if _, err := newTestPackStore(t, client).PutBlockBatch(ctx, foreign); err != nil {
		t.Fatal(err)
	}

	// A cold index-only store misses the foreign blocks with no request.
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	open := func() (*PackStore, *requestCounter) {
		objs := &requestCounter{ObjectStore: client}
		s := NewPackStore(logrus.NewEntry(logrus.New()), objs, "bucket", "p/", &PackStoreOptions{
			KeepPacks: true,
			Index:     kv,
			IndexOnly: true,
		})
		t.Cleanup(s.Close)
		return s, objs
	}
	store, objs := open()
	for _, ref := range foreignRefs {
		if data, found, err := store.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("cold foreign read = %q, %v, %v", data, found, err)
		}
		if exists, err := store.GetBlockExists(ctx, ref); err != nil || exists {
			t.Fatalf("cold foreign exists = %v, %v", exists, err)
		}
	}
	if n := objs.requests.Load(); n != 0 {
		t.Fatalf("cold misses made %d requests, want none", n)
	}

	// The store's own pack reads back.
	own, ownRefs := batch("own", 10)
	if _, err := store.PutBlockBatch(ctx, own); err != nil {
		t.Fatal(err)
	}
	for i, ref := range ownRefs {
		if data, found, err := store.GetBlock(ctx, ref); err != nil || !found || string(data) != string(own[i].Data) {
			t.Fatalf("own read = %q, %v, %v", data, found, err)
		}
	}

	// A warm restart reads its own pack and still misses the foreign one
	// without listing or reading it.
	store.Close()
	store, objs = open()
	for i, ref := range ownRefs {
		if data, found, err := store.GetBlock(ctx, ref); err != nil || !found || string(data) != string(own[i].Data) {
			t.Fatalf("warm own read = %q, %v, %v", data, found, err)
		}
	}
	read := objs.requests.Load()
	for _, ref := range foreignRefs {
		if _, found, err := store.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("warm foreign read = %v, %v", found, err)
		}
	}
	if n := objs.requests.Load() - read; n != 0 {
		t.Fatalf("warm misses made %d requests, want none", n)
	}

	// A pack written after the restart takes a new number: were the first
	// pack's number reused, a third start would place one pack's blocks in
	// the other and miss them.
	// The write is the restarted store's first call, before any read loads
	// the table.
	store.Close()
	store, _ = open()
	later, laterRefs := batch("later", 10)
	if _, err := store.PutBlockBatch(ctx, later); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, _ = open()
	for i, ref := range append(ownRefs, laterRefs...) {
		want := append(own, later...)[i].Data
		if data, found, err := store.GetBlock(ctx, ref); err != nil || !found || string(data) != string(want) {
			t.Fatalf("third start read = %q, %v, %v", data, found, err)
		}
	}
}

// TestPackStoreIndexOnlyRestored checks that an index-only store opened on a
// copy of its index taken before its last pack reads that pack once the copy
// is marked with RequestPackListing, that the listing drops no pack the copy
// holds even when the listing lacks its entry, and that the mark clears so the
// next start lists nothing.
func TestPackStoreIndexOnlyRestored(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	batch := func(name string) ([]*block.PutBatchEntry, []*block.BlockRef) {
		var entries []*block.PutBatchEntry
		var refs []*block.BlockRef
		for i := range 10 {
			data := []byte(fmt.Sprintf("%s block %d", name, i))
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data})
			refs = append(refs, ref)
		}
		return entries, refs
	}
	open := func(kv kvtx.Store) (*PackStore, *requestCounter) {
		objs := &requestCounter{ObjectStore: client}
		s := NewPackStore(logrus.NewEntry(logrus.New()), objs, "bucket", "p/", &PackStoreOptions{
			KeepPacks: true,
			Index:     kv,
			IndexOnly: true,
		})
		t.Cleanup(s.Close)
		return s, objs
	}
	readAll := func(s *PackStore, entries []*block.PutBatchEntry, refs []*block.BlockRef) {
		t.Helper()
		for i, ref := range refs {
			if data, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(data) != string(entries[i].Data) {
				t.Fatalf("read = %q, %v, %v", data, found, err)
			}
		}
	}
	entryIDs := func() map[string]bool {
		ids := make(map[string]bool)
		err := client.ListObjects(ctx, "bucket", "p/"+entryDir, func(key string, _ int64) error {
			ids[key] = true
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}

	// The store writes a pack, its index is backed up, and it writes another.
	live := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	store, _ := open(live)
	early, earlyRefs := batch("early")
	if _, err := store.PutBlockBatch(ctx, early); err != nil {
		t.Fatal(err)
	}
	earlyEntries := entryIDs()
	backup := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	copyKV(t, live, backup)
	late, lateRefs := batch("late")
	if _, err := store.PutBlockBatch(ctx, late); err != nil {
		t.Fatal(err)
	}
	store.Close()

	// Without the mark the restored copy misses the later pack.
	unmarked := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	copyKV(t, backup, unmarked)
	store, _ = open(unmarked)
	if _, found, err := store.GetBlock(ctx, lateRefs[0]); err != nil || found {
		t.Fatalf("unmarked late read = %v, %v", found, err)
	}
	store.Close()

	// The early pack's entry leaves the bucket; the restored index still
	// holds it, so its blocks must keep reading.
	for key := range earlyEntries {
		if err := client.DeleteObject(ctx, "bucket", key); err != nil {
			t.Fatal(err)
		}
	}

	// Marked, the restored copy reads both packs.
	if err := RequestPackListing(ctx, backup); err != nil {
		t.Fatal(err)
	}
	store, _ = open(backup)
	readAll(store, late, lateRefs)
	readAll(store, early, earlyRefs)
	store.Close()

	// The mark is gone: the next start lists nothing and reads both packs.
	store, objs := open(backup)
	readAll(store, early, earlyRefs)
	readAll(store, late, lateRefs)
	if n := objs.lists.Load(); n != 0 {
		t.Fatalf("restart after the listing listed %d times, want none", n)
	}
}

// copyKV copies every key of src into dst.
func copyKV(t *testing.T, src, dst kvtx.Store) {
	t.Helper()
	ctx := t.Context()
	rtx, err := src.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	wtx, err := dst.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Discard()
	err = rtx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		return wtx.Set(ctx, bytes.Clone(key), bytes.Clone(value))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
