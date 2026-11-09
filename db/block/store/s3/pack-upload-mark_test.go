//go:build !tinygo

package block_store_s3

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/sirupsen/logrus"
)

// failEntryPuts is an object store whose entry writes fail, as a process
// stopped right after it uploaded a pack leaves the bucket.
type failEntryPuts struct {
	ObjectStore
}

// PutObject fails for an entry and writes anything else.
func (f failEntryPuts) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if strings.Contains(key, entryDir) {
		return errors.New("stopped")
	}
	return f.ObjectStore.PutObject(ctx, bucket, key, data, contentType)
}

// writeLimit is a store whose write transactions fail once left reaches zero,
// as a process stopped there leaves it. A negative left never stops.
type writeLimit struct {
	kvtx.Store
	left atomic.Int64
}

// NewTransaction opens the transaction unless the limit stops a write.
func (w *writeLimit) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write && w.left.Load() >= 0 && w.left.Add(-1) < 0 {
		return nil, errors.New("stopped")
	}
	return w.Store.NewTransaction(ctx, write)
}

// TestPackUploadMarker stops a publish after the pack's upload and before its
// first index slice, once at the entry's write and once at the index's, then
// reopens the store over the same bucket and index. The reopened store adopts
// the uploaded pack from its tail, so the index places every block, each
// reads back, and the bucket holds the one pack, never a second copy. A marker
// whose pack never reached the bucket is deleted.
func TestPackUploadMarker(t *testing.T) {
	for _, tc := range []struct {
		name string
		// stopEntry fails the entry's write; otherwise the index's first
		// slice fails.
		stopEntry bool
		// soleWriter loads from the index table; otherwise the first load
		// also lists the bucket.
		soleWriter bool
	}{
		{name: "entry", stopEntry: true, soleWriter: true},
		{name: "index", stopEntry: false, soleWriter: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			bucket, client := newFakeBucket(t, nil)
			kv := &writeLimit{Store: hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]())}
			kv.left.Store(-1)
			open := func(objects ObjectStore) *PackStore {
				s := NewPackStore(logrus.NewEntry(logrus.New()), objects, "bucket", "p/", &PackStoreOptions{
					KeepPacks:  true,
					Index:      kv,
					SoleWriter: tc.soleWriter,
				})
				t.Cleanup(s.Close)
				return s
			}

			// Build a batch of blocks.
			var batch []*block.PutBatchEntry
			var refs []*block.BlockRef
			for i := range 5 {
				data := []byte("block" + strconv.Itoa(i))
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				refs = append(refs, ref)
				batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
			}

			// Publish it and stop after the pack's upload.
			var objects ObjectStore = client
			if tc.stopEntry {
				objects = failEntryPuts{ObjectStore: client}
			}
			first := open(objects)
			if err := first.Preload(ctx); err != nil {
				t.Fatal(err)
			}
			if !tc.stopEntry {
				// Allow the marker's transaction and stop the first slice.
				kv.left.Store(1)
			}
			if _, err := first.PutBlockBatch(ctx, batch); err == nil {
				t.Fatal("publish did not stop")
			}

			// Leave a marker for a pack whose upload never reached the bucket.
			absent := &packfile.PackfileEntry{Id: identity.NewPackID(), SizeBytes: 4096, BlockCount: 3}
			kv.left.Store(-1)
			if err := first.index.markUpload(ctx, absent); err != nil {
				t.Fatal(err)
			}
			first.Close()
			packs := countKeys(bucket, "p/"+packDir)
			if packs != 1 || rowCount(t, kv, indexUploadPrefix) != 2 || rowCount(t, kv, indexPackPrefix) != 0 {
				t.Fatalf("after the stop: %d packs, %d markers, %d pack rows", packs, rowCount(t, kv, indexUploadPrefix), rowCount(t, kv, indexPackPrefix))
			}
			puts := bucketPuts(bucket)

			// Reopen: the index places every block, and each reads back.
			second := open(client)
			located, err := second.LocateBlocks(ctx, refs)
			if err != nil {
				t.Fatal(err)
			}
			for i, ok := range located {
				if !ok {
					t.Fatalf("block %d not located after the restart", i)
				}
			}
			for i, ref := range refs {
				data, found, err := second.GetBlock(ctx, ref)
				if err != nil || !found || string(data) != "block"+strconv.Itoa(i) {
					t.Fatalf("block %d = %q, %v, %v", i, data, found, err)
				}
			}

			// The bucket holds the one pack and its entry, the restart wrote
			// only the entry, and both markers are gone.
			if n := countKeys(bucket, "p/"+packDir); n != 1 {
				t.Fatalf("%d packs after the restart; want the one uploaded", n)
			}
			if n := countKeys(bucket, "p/"+entryDir); n != 1 {
				t.Fatalf("%d entries after the restart; want 1", n)
			}
			if n := bucketPuts(bucket) - puts; n != 1 {
				t.Fatalf("the restart wrote %d objects; want the entry alone", n)
			}
			if n := rowCount(t, kv, indexUploadPrefix); n != 0 {
				t.Fatalf("%d markers after the restart", n)
			}

			// A third start, listing or not, keeps the pack.
			third := open(client)
			if located, err := third.LocateBlocks(ctx, refs); err != nil || !located[0] || !located[len(located)-1] {
				t.Fatalf("third start located %v: %v", located, err)
			}
			if n := countKeys(bucket, "p/"+packDir); n != 1 {
				t.Fatalf("%d packs after the third start", n)
			}
		})
	}
}

// countKeys counts the bucket's objects under prefix.
func countKeys(b *fakeBucket, prefix string) int {
	n := 0
	for _, key := range b.keys() {
		if strings.HasPrefix(key, prefix) {
			n++
		}
	}
	return n
}

// bucketPuts reads the bucket's PUT count.
func bucketPuts(b *fakeBucket) int {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.puts
}
