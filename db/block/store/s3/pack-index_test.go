package block_store_s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// countingObjects counts the entry reads and the pack range reads of an
// object store.
type countingObjects struct {
	ObjectStore
	entryReads atomic.Int64
	rangeReads atomic.Int64
}

func (c *countingObjects) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if strings.Contains(key, entryDir) {
		c.entryReads.Add(1)
	}
	return c.ObjectStore.GetObject(ctx, bucket, key)
}

func (c *countingObjects) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	c.rangeReads.Add(1)
	return c.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

// TestPackStoreIndex walks the location index through a cold start, a lookup, a
// warm restart, a new pack, a removed pack and a miss.
func TestPackStoreIndex(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)

	// Write three packs with a store that keeps no index.
	writer := newTestPackStore(t, client)
	var refs []*block.BlockRef
	putPack := func(n int) []*block.BlockRef {
		var batch []*block.PutBatchEntry
		var out []*block.BlockRef
		for i := range 20 {
			data := []byte(fmt.Sprintf("pack %d block %d", n, i))
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data})
			out = append(out, ref)
		}
		if _, err := writer.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for n := range 3 {
		refs = append(refs, putPack(n)...)
	}

	// Open a store with an index: the first read lists and reads the entries,
	// and the background indexer covers the three packs.
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	open := func() (*PackStore, *countingObjects) {
		objs := &countingObjects{ObjectStore: client}
		s := NewPackStore(logrus.NewEntry(logrus.New()), objs, "bucket", "p/", &PackStoreOptions{
			KeepPacks:       true,
			Index:           kv,
			RefreshInterval: 20 * time.Millisecond,
		})
		return s, objs
	}
	store, objs := open()
	if data, found, err := store.GetBlock(ctx, refs[0]); err != nil || !found || string(data) != "pack 0 block 0" {
		t.Fatalf("cold read = %q, %v, %v", data, found, err)
	}
	if got := objs.entryReads.Load(); got != 3 {
		t.Fatalf("cold start read %d entries, want 3", got)
	}
	waitFor(t, "cold start indexes the packs", func() bool { return len(store.index.pendingIDs()) == 0 })

	// A lookup finds the pack through the index, without a bloom filter.
	for _, ref := range refs {
		id, found, err := store.index.Locate(ctx, hashKey(ref))
		if err != nil || !found || !store.index.Indexed(id) {
			t.Fatalf("locate = %q, %v, %v", id, found, err)
		}
		if data, found, err := store.GetBlock(ctx, ref); err != nil || !found || len(data) == 0 {
			t.Fatalf("indexed read = %v, %v", found, err)
		}
	}

	// A pack another writer adds meanwhile, then a restart: the warm start
	// reads the cached entries and only the new entry from the bucket.
	store.Close()
	refs = append(refs, putPack(3)...)
	store, objs = open()
	t.Cleanup(store.Close)
	if data, found, err := store.GetBlock(ctx, refs[1]); err != nil || !found || len(data) == 0 {
		t.Fatalf("warm read = %v, %v", found, err)
	}
	waitFor(t, "warm start lists the new pack", func() bool { return store.index.Indexed(packOf(t, store, refs[len(refs)-1])) })
	if got := objs.entryReads.Load(); got != 1 {
		t.Fatalf("warm start read %d entries, want only the new one", got)
	}

	// A pack removed from the listing leaves the index.
	oldID, _, _ := store.index.Locate(ctx, hashKey(refs[0]))
	if err := client.DeleteObject(ctx, "bucket", "p/"+entryDir+oldID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "removed pack leaves the index", func() bool {
		id, found, _ := store.index.Locate(ctx, hashKey(refs[0]))
		return !found && id == "" && !store.index.Indexed(oldID)
	})
	if exists, err := store.GetBlockExists(ctx, refs[0]); err != nil || exists {
		t.Fatalf("removed pack block exists = %v, %v", exists, err)
	}
	if got := rowCount(t, kv, indexEntryPrefix); got != 3 {
		t.Fatalf("cache holds %d entries, want 3", got)
	}

	// With every pack indexed, a miss reads no pack.
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	reads := objs.rangeReads.Load()
	if exists, err := store.GetBlockExists(ctx, missing); err != nil || exists {
		t.Fatalf("missing exists = %v, %v", exists, err)
	}
	if objs.rangeReads.Load() != reads {
		t.Fatal("a miss with a complete index read a pack")
	}
}

func hashKey(ref *block.BlockRef) []byte {
	return packfile.BlockKey(ref.GetHash())
}

func packOf(t *testing.T, s *PackStore, ref *block.BlockRef) string {
	id, _, err := s.index.Locate(t.Context(), hashKey(ref))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func rowCount(t *testing.T, kv kvtx.Store, prefix string) int {
	tx, err := kv.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	n := 0
	if err := tx.ScanPrefixKeys(t.Context(), []byte(prefix), func([]byte) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
