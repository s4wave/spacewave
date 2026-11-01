package block_store_s3

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// TestPackStoreMergesTail writes short batches into a store that keeps its
// packs and merges its tail, while a second store reads the packfiles the
// merges delete. Every block stays readable through each merge, a full tail
// starts a new packfile, and an unreadable tail is kept rather than replaced.
func TestPackStoreMergesTail(t *testing.T) {
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	opts := &PackStoreOptions{KeepPacks: true, MergeTailBytes: 4 << 10}
	store := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", opts)
	t.Cleanup(store.Close)
	packs := func() []string {
		bucket.mtx.Lock()
		defer bucket.mtx.Unlock()
		var keys []string
		for key := range bucket.objects {
			if strings.HasPrefix(key, "p/"+packDir) {
				keys = append(keys, key)
			}
		}
		return keys
	}

	// A reader lists the first packfile, which every later merge replaces.
	refs := putBlocks(t, store, "block", 1)
	reader := newTestPackStore(t, client)
	checkBlocks(t, reader, "block", refs)

	// Forty one-block batches, and one repeated block, merge into one packfile.
	// The reader finds the packfile it listed deleted, lists again, and reads
	// the block from the merged packfile.
	refs = putBlocks(t, store, "block", 40)
	if got := packs(); len(got) != 1 {
		t.Fatalf("bucket holds %d packfiles after short batches; want 1", len(got))
	}
	checkBlocks(t, reader, "block", refs[:1])

	// A block merged first without its refs gains them when it is written
	// again with them.
	parent := []byte("parent")
	if _, _, err := store.PutBlock(ctx, parent, nil); err != nil {
		t.Fatal(err)
	}
	parentRef, _, err := store.PutBlock(ctx, parent, &block.PutOpts{Refs: refs[:1], RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := newTestPackStore(t, client).GetStoredBlock(ctx, parentRef)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(refs[0]) {
		t.Fatalf("stored parent = %v; want the refs of its second write", stored)
	}

	// A block that fills the tail past the merge size merges once, and the next
	// short batch starts a new packfile.
	if _, _, err := store.PutBlock(ctx, bytes.Repeat([]byte{'x'}, 5<<10), nil); err != nil {
		t.Fatal(err)
	}
	if got := packs(); len(got) != 1 {
		t.Fatalf("bucket holds %d packfiles after filling the tail; want 1", len(got))
	}
	more := putBlocks(t, store, "more", 1)
	tails := packs()
	if len(tails) != 2 {
		t.Fatalf("bucket holds %d packfiles after a full tail; want 2", len(tails))
	}

	// Corrupt the new tail: the next batch is written beside it, and the
	// unreadable tail is kept.
	tail := "p/" + packDir + store.pickTail(packPayloadBytes).GetId()
	bucket.mtx.Lock()
	good := bucket.objects[tail]
	bucket.objects[tail] = "not a packfile"
	bucket.mtx.Unlock()
	late := putBlocks(t, store, "late", 1)
	if got := packs(); len(got) != 3 {
		t.Fatalf("bucket holds %d packfiles after a corrupt tail; want 3", len(got))
	}
	bucket.mtx.Lock()
	bucket.objects[tail] = good
	bucket.mtx.Unlock()

	// A restarted writer that has not read yet merges into the tail the
	// first store left.
	restarted := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", opts)
	t.Cleanup(restarted.Close)
	again := putBlocks(t, restarted, "again", 1)
	if got := packs(); len(got) != 3 {
		t.Fatalf("bucket holds %d packfiles after a restarted write; want 3", len(got))
	}

	// A fresh store reads every block from the packfiles that remain.
	fresh := newTestPackStore(t, client)
	checkBlocks(t, fresh, "block", refs)
	checkBlocks(t, fresh, "more", more)
	checkBlocks(t, fresh, "late", late)
	checkBlocks(t, fresh, "again", again)

	// A reader whose listing predates a merge, and whose entry read finds the
	// replaced entry deleted, lists again and finds the merged packfile.
	bucket, client = newFakeBucket(t, nil)
	writer := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", opts)
	t.Cleanup(writer.Close)
	first := putBlocks(t, writer, "first", 1)
	var merged atomic.Int32
	mergeAfter := func(match func(r *http.Request) bool, count int32) {
		var busy atomic.Bool
		merged.Store(0)
		bucket.mtx.Lock()
		bucket.after = func(r *http.Request) {
			if !match(r) || merged.Load() >= count || !busy.CompareAndSwap(false, true) {
				return
			}
			defer busy.Store(false)
			n := merged.Add(1)
			putBlocks(t, writer, "merge"+strconv.Itoa(int(n)), 1)
		}
		bucket.mtx.Unlock()
	}
	entryDir := "p/" + entryDir
	mergeAfter(func(r *http.Request) bool {
		return r.URL.Query().Get("prefix") == entryDir
	}, 1)
	checkBlocks(t, newTestPackStore(t, client), "first", first)
	if got := merged.Load(); got != 1 {
		t.Fatalf("%d merges ran after the listing; want 1", got)
	}

	// A reader crosses more merges than any fixed bound: each entry it reads
	// names a packfile a merge deletes before the reader reads the block.
	mergeAfter(func(r *http.Request) bool {
		return strings.HasPrefix(r.URL.Path, "/bucket/"+entryDir)
	}, 8)
	checkBlocks(t, newTestPackStore(t, client), "first", first)
	if got := merged.Load(); got != 8 {
		t.Fatalf("%d merges ran after entry reads; want 8", got)
	}
	bucket.mtx.Lock()
	bucket.after = nil
	bucket.mtx.Unlock()
	if got := packs(); len(got) != 1 {
		t.Fatalf("bucket holds %d packfiles after crossing merges; want 1", len(got))
	}
}
