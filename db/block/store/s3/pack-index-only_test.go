package block_store_s3

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// requestCounter counts the reads and listings of an object store.
type requestCounter struct {
	ObjectStore
	requests atomic.Int64
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
