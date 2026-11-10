//go:build !tinygo

package block_store_s3

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// TestAdoptPack adopts a pack another writer uploaded into a store that never
// lists the bucket, and tries to make it register a pack it should not.
func TestAdoptPack(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)

	// The local store owns one pack, so it never lists the bucket again.
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	open := func() *PackStore {
		s := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
		t.Cleanup(s.Close)
		return s
	}
	local := open()
	adoptBlocks(ctx, t, local, "own", 1)

	// The node uploads a pack the local store has not heard of.
	node := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, node, "node", 3)
	entry := nodeEntry(ctx, t, node)
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i] = packfile.BlockKey(ref.GetHash())
	}
	located := func(s *PackStore) bool {
		found, err := s.LocateBlocks(ctx, refs)
		if err != nil {
			t.Fatal(err)
		}
		return found[0] && found[1] && found[2]
	}
	if located(local) {
		t.Fatal("located a pack before adopting it")
	}

	// A report that does not match the pack is refused and registers nothing:
	// a missing key, an extra key, a wrong size and a pack that is not there.
	short := keys[:2]
	extra := append(append([][]byte{}, keys...), packfile.BlockKey(adoptBlocks(ctx, t, newTestPackStore(t, client), "other", 1)[0].GetHash()))
	wrongSize := entry.CloneVT()
	wrongSize.SizeBytes--
	absent := entry.CloneVT()
	absent.Id = strings.Repeat("0", len(entry.GetId()))
	for name, bad := range map[string]func() error{
		"missing key": func() error { return local.AdoptPack(ctx, entry, short) },
		"extra key":   func() error { return local.AdoptPack(ctx, entry, extra) },
		"wrong size":  func() error { return local.AdoptPack(ctx, wrongSize, keys) },
		"absent pack": func() error { return local.AdoptPack(ctx, absent, keys) },
	} {
		if err := bad(); err == nil {
			t.Fatalf("%s: adopted", name)
		}
		if found, err := local.LocateBlocks(ctx, refs[:1]); err != nil || found[0] {
			t.Fatalf("%s: registered the block: %v %v", name, found, err)
		}
	}

	// A true report registers the pack, and a repeat changes nothing.
	for range 2 {
		if err := local.AdoptPack(ctx, entry, keys); err != nil {
			t.Fatal(err)
		}
	}
	if !located(local) {
		t.Fatal("adopted pack not located")
	}
	if data, found, err := local.GetBlock(ctx, refs[1]); err != nil || !found || string(data) != "node block 1" {
		t.Fatalf("adopted block read = %q, %v, %v", data, found, err)
	}

	// The adoption survives a restart with no listing of the bucket.
	local.Close()
	restarted := open()
	if !located(restarted) {
		t.Fatal("adopted pack lost by a restart")
	}
	if data, found, err := restarted.GetBlock(ctx, refs[2]); err != nil || !found || string(data) != "node block 2" {
		t.Fatalf("read after restart = %q, %v, %v", data, found, err)
	}
}

// TestAdoptBucketPack adopts a pack with no copy at hand to report its keys,
// taking them from the pack's index in the bucket, and tries to make it
// register a pack the bucket does not hold as its entry says.
func TestAdoptBucketPack(t *testing.T) {
	// A local store that never lists the bucket, and a pack from the node.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	local := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
	t.Cleanup(local.Close)
	adoptBlocks(ctx, t, local, "own", 1)
	node := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, node, "node", 3)
	entry := nodeEntry(ctx, t, node)
	registered := func() bool {
		found, err := local.LocateBlocks(ctx, refs)
		if err != nil {
			t.Fatal(err)
		}
		return found[0] || found[1] || found[2]
	}

	// Count the reads of the pack's body.
	key := "p/" + packDir + entry.GetId()
	var gets atomic.Int32
	bucket.after = func(r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+key) {
			gets.Add(1)
		}
	}

	// An entry the bucket object does not match is refused and registers
	// nothing: a wrong size, a wrong block count and a pack that is not there.
	wrongSize := entry.CloneVT()
	wrongSize.SizeBytes++
	wrongCount := entry.CloneVT()
	wrongCount.BlockCount++
	gone := newTestPackStore(t, client)
	adoptBlocks(ctx, t, gone, "gone", 2)
	var absent *packfile.PackfileEntry
	if err := gone.loadEntries(ctx); err != nil {
		t.Fatal(err)
	}
	gone.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, e := range gone.entries {
			if e.GetBlockCount() == 2 {
				absent = e
			}
		}
	})
	if absent == nil {
		t.Fatal("no two-block pack")
	}
	bucket.mtx.Lock()
	delete(bucket.objects, "p/"+packDir+absent.GetId())
	bucket.mtx.Unlock()
	for name, bad := range map[string]*packfile.PackfileEntry{
		"wrong size":  wrongSize,
		"wrong count": wrongCount,
		"absent pack": absent,
	} {
		if err := local.AdoptBucketPack(ctx, bad); err == nil {
			t.Fatalf("%s: adopted", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
		if registered() {
			t.Fatalf("%s: registered a block", name)
		}
		if held, err := local.HasPackfile(ctx, bad.GetId()); err != nil || held {
			t.Fatalf("%s: pack counts as held: %v %v", name, held, err)
		}
	}
	if gets.Load() != 1 {
		t.Fatalf("the refused entries read the pack %d times, want once for the wrong count", gets.Load())
	}

	// The true entry registers the pack with the keys its index lists, reading
	// the index once, and a repeat reads nothing.
	gets.Store(0)
	if err := local.AdoptBucketPack(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if gets.Load() != 1 {
		t.Fatalf("adoption read the pack %d times, want once", gets.Load())
	}
	if err := local.AdoptBucketPack(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if gets.Load() != 1 {
		t.Fatalf("a repeat read the pack again: %d reads", gets.Load())
	}
	found, err := local.LocateBlocks(ctx, refs)
	if err != nil || !found[0] || !found[1] || !found[2] {
		t.Fatalf("located %v after adoption: %v", found, err)
	}
	if data, ok, err := local.GetBlock(ctx, refs[1]); err != nil || !ok || string(data) != "node block 1" {
		t.Fatalf("adopted block read = %q, %v, %v", data, ok, err)
	}

}

// countingCache is a RangeCache that answers data, counting its reads.
type countingCache struct {
	data  func(id string, off int64, length int) ([]byte, bool)
	reads atomic.Int32
}

// ReadCached answers from data.
func (c *countingCache) ReadCached(_ context.Context, id string, off int64, length int) ([]byte, bool) {
	c.reads.Add(1)
	return c.data(id, off, length)
}

// TestReadCacheNeverVouches gives a store a read cache that holds a pack the
// bucket no longer has, and bytes that are wrong. Nothing about what the bucket
// holds may come from it, and no bad answer may fail a read the bucket can
// serve.
func TestReadCacheNeverVouches(t *testing.T) {
	// A store with a cache, and a block with an extent row in its index.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	cache := &countingCache{}
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	store := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true, ExactReads: true, ReadCache: cache})
	t.Cleanup(store.Close)
	adoptBlocks(ctx, t, store, "own", 1)
	node := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, node, "node", 3)
	entry := nodeEntry(ctx, t, node)
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i] = packfile.BlockKey(ref.GetHash())
	}

	// A pack gone from the bucket can't be adopted, whatever the cache holds.
	key := "p/" + packDir + entry.GetId()
	bucket.mtx.Lock()
	saved := bucket.objects[key]
	delete(bucket.objects, key)
	bucket.mtx.Unlock()
	if saved == "" {
		t.Fatalf("no object %s", key)
	}
	cache.data = func(_ string, off int64, length int) ([]byte, bool) {
		if off+int64(length) > int64(len(saved)) {
			return nil, false
		}
		return []byte(saved[off : off+int64(length)]), true
	}
	if err := store.AdoptPack(ctx, entry, keys); err == nil {
		t.Fatal("adopted a pack only the cache holds")
	}
	if cache.reads.Load() != 0 {
		t.Fatalf("adoption asked the cache %d times", cache.reads.Load())
	}
	bucket.mtx.Lock()
	bucket.objects[key] = saved
	bucket.mtx.Unlock()
	if err := store.AdoptPack(ctx, entry, keys); err != nil {
		t.Fatal(err)
	}

	// Existence and location never ask it.
	if found, err := store.GetBlockExists(ctx, refs[0]); err != nil || !found {
		t.Fatalf("exists = %v, %v", found, err)
	}
	if cache.reads.Load() != 0 {
		t.Fatalf("an existence check asked the cache %d times", cache.reads.Load())
	}

	// Adoption queues the pack's extents, which reach the block's row soon.
	waitFor(t, "the adopted pack's row holds its extent", func() bool {
		_, _, _, ok, err := store.index.locateExtent(ctx, keys[0])
		return err == nil && ok
	})

	// Whatever the cache answers, the read is right, and a true answer is used.
	for name, answer := range map[string]func(string, int64, int) ([]byte, bool){
		"missing": func(string, int64, int) ([]byte, bool) { return nil, false },
		"zeroed":  func(_ string, _ int64, n int) ([]byte, bool) { return make([]byte, n), true },
		"short":   func(_ string, _ int64, n int) ([]byte, bool) { return make([]byte, n/2), true },
	} {
		cache.data = answer
		if data, found, err := store.GetBlock(ctx, refs[0]); err != nil || !found || string(data) != "node block 0" {
			t.Fatalf("%s: read = %q, %v, %v", name, data, found, err)
		}
	}
	if cache.reads.Load() != 3 {
		t.Fatalf("the cache took %d reads, want one for each answer", cache.reads.Load())
	}
	cache.data = func(_ string, off int64, length int) ([]byte, bool) {
		return []byte(saved[off : off+int64(length)]), true
	}
	bucket.mtx.Lock()
	delete(bucket.objects, key)
	bucket.mtx.Unlock()
	if data, found, err := store.GetBlock(ctx, refs[0]); err != nil || !found || string(data) != "node block 0" {
		t.Fatalf("read from the cache alone = %q, %v, %v", data, found, err)
	}
}

// failingIndex fails its write transactions while failing is set.
type failingIndex struct {
	kvtx.Store
	failing atomic.Bool
}

// NewTransaction refuses a write while failing is set.
func (f *failingIndex) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write && f.failing.Load() {
		return nil, errors.New("index write failed")
	}
	return f.Store.NewTransaction(ctx, write)
}

// TestAdoptFailedIndexWrite checks that a pack whose index write failed does
// not count as registered, so its repeat registers it rather than reporting a
// success that left it unreadable.
func TestAdoptFailedIndexWrite(t *testing.T) {
	// A local store with an index that can fail, and a pack from the node.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := &failingIndex{Store: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	local := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
	t.Cleanup(local.Close)
	adoptBlocks(ctx, t, local, "own", 1)
	node := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, node, "node", 3)
	entry := nodeEntry(ctx, t, node)
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i] = packfile.BlockKey(ref.GetHash())
	}

	// The failed adoption reports failure and registers nothing.
	kv.failing.Store(true)
	if err := local.AdoptPack(ctx, entry, keys); err == nil {
		t.Fatal("adoption succeeded over a failed index write")
	}
	kv.failing.Store(false)
	if held, err := local.HasPackfile(ctx, entry.GetId()); err != nil || held {
		t.Fatalf("failed pack counts as held: %v %v", held, err)
	}

	// The repeat registers it for real.
	if err := local.AdoptPack(ctx, entry, keys); err != nil {
		t.Fatal(err)
	}
	found, err := local.LocateBlocks(ctx, refs)
	if err != nil || !found[0] || !found[1] || !found[2] {
		t.Fatalf("located %v after the repeat: %v", found, err)
	}
}

// adoptBlocks writes count blocks named after name as one pack.
func adoptBlocks(ctx context.Context, t *testing.T, s *PackStore, name string, count int) []*block.BlockRef {
	t.Helper()
	var batch []*block.PutBatchEntry
	var refs []*block.BlockRef
	for i := range count {
		data := []byte(fmt.Sprintf("%s block %d", name, i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
		refs = append(refs, ref)
	}
	if _, err := s.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	return refs
}

// nodeEntry returns the three-block entry s wrote.
func nodeEntry(ctx context.Context, t *testing.T, s *PackStore) *packfile.PackfileEntry {
	t.Helper()
	if err := s.loadEntries(ctx); err != nil {
		t.Fatal(err)
	}
	var found *packfile.PackfileEntry
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, entry := range s.entries {
			if entry.GetBlockCount() == 3 {
				found = entry
			}
		}
	})
	if found == nil {
		t.Fatal("no three-block pack")
	}
	return found
}
