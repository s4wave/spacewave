package block_store_s3

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
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
	putPack := func(n, count int) []*block.BlockRef {
		var batch []*block.PutBatchEntry
		var out []*block.BlockRef
		for i := range count {
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
		refs = append(refs, putPack(n, 20)...)
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

	// The indexer keeps each pack's index tail, though only one pack was read.
	waitFor(t, "the indexer keeps the tails", func() bool { return rowCount(t, kv, indexTailPrefix) == 3 })

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
	refs = append(refs, putPack(3, 20)...)
	store, objs = open()
	t.Cleanup(store.Close)
	if data, found, err := store.GetBlock(ctx, refs[1]); err != nil || !found || len(data) == 0 {
		t.Fatalf("warm read = %v, %v", found, err)
	}
	waitFor(t, "warm start lists the new pack", func() bool { return store.index.Indexed(packOf(t, store, refs[len(refs)-1])) })
	if got := objs.entryReads.Load(); got != 1 {
		t.Fatalf("warm start read %d entries, want only the new one", got)
	}

	// A pack removed from the listing leaves the index; its rows still say the
	// block was placed, with no pack.
	oldID, _, _ := store.index.Locate(ctx, hashKey(refs[0]))
	store.index.mtx.RLock()
	oldNum := store.index.rows[oldID].num
	store.index.mtx.RUnlock()
	if err := client.DeleteObject(ctx, "bucket", "p/"+entryDir+oldID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "removed pack leaves the index", func() bool {
		id, found, _ := store.index.Locate(ctx, hashKey(refs[0]))
		return found && id == "" && !store.index.Indexed(oldID)
	})
	if exists, err := store.GetBlockExists(ctx, refs[0]); err != nil || exists {
		t.Fatalf("removed pack block exists = %v, %v", exists, err)
	}
	if got := rowCount(t, kv, indexEntryPrefix); got != 3 {
		t.Fatalf("cache holds %d entries, want 3", got)
	}

	// A row that names the removed pack shadows a block a live, indexed pack
	// holds, as when the indexer finished a pack a tail merge had replaced:
	// the read still finds it through the filters.
	shadowed := refs[len(refs)-1]
	tx, err := kv.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, hashRowKey(hashKey(shadowed)), binary.BigEndian.AppendUint32(nil, oldNum)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	if data, found, err := store.GetBlock(ctx, shadowed); err != nil || !found || string(data) != "pack 3 block 19" {
		t.Fatalf("shadowed read = %q, %v, %v", data, found, err)
	}

	// The indexer does not replace a row that names another live pack.
	live := packOf(t, store, refs[len(refs)-2])
	other := packOf(t, store, refs[20])
	if other == live || other == "" {
		t.Fatalf("pack of refs[20] = %q, want a live pack other than %s", other, live)
	}
	if _, err := store.index.writeKeys(ctx, other, func(fn func([]byte) error) error {
		return fn(hashKey(refs[len(refs)-2]))
	}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := packOf(t, store, refs[len(refs)-2]); got != live {
		t.Fatalf("indexer moved a live row from %s to %s", live, got)
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

	// Re-index more than a batch of distinct keys another live pack holds.
	// Preserving every row must still release the writer for foreground work.
	duplicates := putPack(4, indexBatchKeys+1)
	waitFor(t, "duplicate pack indexed", func() bool {
		return store.index.Indexed(packOf(t, store, duplicates[0]))
	})
	livePack := packOf(t, store, duplicates[0])
	otherPack := packOf(t, store, refs[20])
	var scanned, pauses int
	written, err := store.index.writeKeys(ctx, otherPack, func(fn func([]byte) error) error {
		for _, ref := range duplicates {
			scanned++
			if err := fn(hashKey(ref)); err != nil {
				return err
			}
		}
		return nil
	}, func(ctx context.Context) error {
		// A batch ends after inspected rows, even when none needed writing.
		pauses++
		if want := min(pauses*indexBatchKeys, len(duplicates)); scanned != want {
			return fmt.Errorf("yield after %d inspected keys, want %d", scanned, want)
		}

		// Admit a real foreground writer while the indexer is yielding.
		admission, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			tx, err := kv.NewTransaction(admission, true)
			if err != nil {
				done <- err
				return
			}
			defer tx.Discard()
			if err := tx.Set(admission, []byte("foreground-test"), []byte("admitted")); err != nil {
				done <- err
				return
			}
			done <- tx.Commit(admission)
		}()
		select {
		case err := <-done:
			return err
		case <-admission.Done():
			return admission.Err()
		}
	})
	if err != nil || written != 0 || pauses != 2 {
		t.Fatalf("preserved batch: writes=%d pauses=%d error=%v", written, pauses, err)
	}
	foreground, err := kv.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := foreground.Get(ctx, []byte("foreground-test"))
	foreground.Discard()
	if err != nil || !found || string(value) != "admitted" {
		t.Fatalf("foreground write = %q, %v, %v", value, found, err)
	}

	// Every preserved key still names its live pack after the bounded scan.
	for _, ref := range duplicates {
		if got := packOf(t, store, ref); got != livePack {
			t.Fatalf("preserved row names %q, want %q", got, livePack)
		}
	}

	// A row that names a live pack lacking the block still reads it: the
	// lookup tests every filter once the located pack misses.
	numOf := func(id string) uint32 {
		store.index.mtx.RLock()
		defer store.index.mtx.RUnlock()
		return store.index.rows[id].num
	}
	misplaced := refs[len(refs)-3]
	setRow(t, kv, misplaced, numOf(otherPack))
	if data, found, err := store.GetBlock(ctx, misplaced); err != nil || !found || string(data) != "pack 3 block 17" {
		t.Fatalf("misplaced read = %q, %v, %v", data, found, err)
	}

	// Removing the newest pack and restarting does not give its number to the
	// next pack, which would point the removed pack's rows at it.
	newestNum := numOf(livePack)
	if err := client.DeleteObject(ctx, "bucket", "p/"+entryDir+livePack); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "newest pack leaves the index", func() bool {
		id, found, _ := store.index.Locate(ctx, hashKey(duplicates[0]))
		return found && id == ""
	})
	store.Close()
	next := putPack(5, 1)
	store, _ = open()
	t.Cleanup(store.Close)
	if _, found, err := store.GetBlock(ctx, refs[25]); err != nil || !found {
		t.Fatalf("read after restart = %v, %v", found, err)
	}
	waitFor(t, "next pack indexed", func() bool {
		id := packOf(t, store, next[0])
		return id != "" && store.index.Indexed(id)
	})
	if got := numOf(packOf(t, store, next[0])); got <= newestNum {
		t.Fatalf("next pack numbered %d, want above the removed %d", got, newestNum)
	}
	if id, found, err := store.index.Locate(ctx, hashKey(duplicates[1])); err != nil || !found || id != "" {
		t.Fatalf("removed pack's row after restart = %q, %v, %v", id, found, err)
	}

	// A pack the indexed store writes keeps its tail as it is indexed.
	tails := rowCount(t, kv, indexTailPrefix)
	own := []byte("written by the indexed store")
	ownRef, err := block.BuildBlockRef(own, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ownRef, Data: own}}); err != nil {
		t.Fatal(err)
	}
	if got := rowCount(t, kv, indexTailPrefix); got != tails+1 {
		t.Fatalf("index holds %d tails after a write, want %d", got, tails+1)
	}
}

// setRow points the h row of ref at pack number num.
func setRow(t *testing.T, kv kvtx.Store, ref *block.BlockRef, num uint32) {
	t.Helper()
	tx, err := kv.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(t.Context(), hashRowKey(hashKey(ref)), binary.BigEndian.AppendUint32(nil, num)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
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

// flakyKV fails its first fail transactions, as a load cancelled with the
// request that started it does.
type flakyKV struct {
	kvtx.Store
	fail atomic.Int64
}

func (f *flakyKV) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if f.fail.Add(-1) >= 0 {
		return nil, context.Canceled
	}
	return f.Store.NewTransaction(ctx, write)
}

// TestPackStoreLazyLookup restarts a store over an index whose first load
// fails: the next read loads the pack table again, and each read publishes
// only the pack the index places its block in, reading its entry from the
// index and no entry from the bucket. A miss the index places nowhere is
// final without publishing every entry.
func TestPackStoreLazyLookup(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	writer := newTestPackStore(t, client)
	var refs [][]*block.BlockRef
	for n := range 3 {
		var batch []*block.PutBatchEntry
		var out []*block.BlockRef
		for i := range 10 {
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
		refs = append(refs, out)
	}
	missing, err := block.BuildBlockRef([]byte("never written"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Index the three packs, then restart over an index whose first
	// transaction fails.
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	var soleWriter bool
	open := func(index kvtx.Store) (*PackStore, *countingObjects) {
		objs := &countingObjects{ObjectStore: client}
		return NewPackStore(logrus.NewEntry(logrus.New()), objs, "bucket", "p/", &PackStoreOptions{
			KeepPacks:       true,
			Index:           index,
			RefreshInterval: time.Hour,
			SoleWriter:      soleWriter,
		}), objs
	}
	store, _ := open(kv)
	if _, found, err := store.GetBlock(ctx, refs[0][0]); err != nil || !found {
		t.Fatalf("cold read = %v, %v", found, err)
	}
	waitFor(t, "cold start indexes the packs", func() bool { return len(store.index.pendingIDs()) == 0 })
	store.Close()
	flaky := &flakyKV{Store: kv}
	flaky.fail.Store(1)
	store, objs := open(flaky)
	t.Cleanup(store.Close)
	if err := store.Preload(ctx); err == nil || store.loaded.Load() {
		t.Fatalf("failed load = %v, loaded %v", err, store.loaded.Load())
	}
	published := func() int {
		var n int
		store.bcast.HoldLock(func(func(), func() <-chan struct{}) { n = len(store.entries) })
		return n
	}

	// The next read loads the table and publishes one pack.
	if data, found, err := store.GetBlock(ctx, refs[1][3]); err != nil || !found || string(data) != "pack 1 block 3" {
		t.Fatalf("read after the failed load = %q, %v, %v", data, found, err)
	}
	if n := published(); n != 1 || store.full.Load() {
		t.Fatalf("published %d entries, full %v, want 1 lazily", n, store.full.Load())
	}

	// A miss is final without publishing the rest.
	if found, err := store.GetBlockExists(ctx, missing); err != nil || found {
		t.Fatalf("miss = %v, %v", found, err)
	}
	if n := published(); n != 1 || store.full.Load() {
		t.Fatalf("miss published %d entries, full %v", n, store.full.Load())
	}

	// Every block reads, each pack published on its first read, and no entry
	// was read from the bucket.
	for _, pack := range refs {
		for _, ref := range pack {
			if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("read = %v, %v", found, err)
			}
		}
	}
	if n := published(); n != 3 || store.full.Load() {
		t.Fatalf("published %d entries, full %v, want 3 lazily", n, store.full.Load())
	}
	if got := objs.entryReads.Load(); got != 0 {
		t.Fatalf("read %d entries from the bucket, want 0", got)
	}
	store.Close()

	// Give pack 0 the number of pack 2, as an index written before its
	// table loaded did. A restart as the sole writer publishes both, so every
	// block reads without loading the rest, and never lists the bucket.
	ids := slices.Sorted(maps.Keys(store.index.rows))
	moved := store.index.rows[ids[0]]
	moved.num = store.index.rows[ids[2]].num
	tx, err := kv.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte(indexPackPrefix+ids[0]), moved.marshal()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	soleWriter = true
	store, _ = open(kv)
	t.Cleanup(store.Close)
	if err := store.Preload(ctx); err != nil {
		t.Fatal(err)
	}
	if n := published(); n != 2 {
		t.Fatalf("published %d entries of a shared number, want 2", n)
	}
	for _, pack := range refs {
		for _, ref := range pack {
			if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("read with a shared number = %v, %v", found, err)
			}
		}
	}
	if store.full.Load() {
		t.Fatal("a shared number loaded every entry")
	}

	// The sole writer's own pack reads back through the index it wrote, and
	// no listing ever ran.
	own, err := block.BuildBlockRef([]byte("sole writer block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: own, Data: []byte("sole writer block")}}); err != nil {
		t.Fatal(err)
	}
	if data, found, err := store.GetBlock(ctx, own); err != nil || !found || string(data) != "sole writer block" {
		t.Fatalf("own write = %q, %v, %v", data, found, err)
	}
	if n := store.listings.Load(); n != 0 {
		t.Fatalf("the sole writer listed the bucket %d times", n)
	}
}

// TestPackStoreLoadSkipsListing checks that a load waiting for the listing
// lock returns once another load completes, rather than waiting out the
// listing that holds the lock, as the refresher's first listing does after a
// start from the cached entries.
func TestPackStoreLoadSkipsListing(t *testing.T) {
	_, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	release, err := store.listMtx.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { done <- store.loadEntries(t.Context()) }()
	select {
	case err := <-done:
		t.Fatalf("load returned before any load completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	store.setLoaded()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("load waited for the listing lock after the entries loaded")
	}
}

// stopAfterWrites is a store whose write transactions fail once it opened
// writes of them, as a process stopped there leaves it.
type stopAfterWrites struct {
	kvtx.Store
	writes, opened int
}

func (s *stopAfterWrites) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		if s.opened == s.writes {
			return nil, errors.New("stopped")
		}
		s.opened++
	}
	return s.Store.NewTransaction(ctx, write)
}

// TestAddWrittenInterrupted stops indexing a pack of three slices after its
// first, and reloads the index: the pack keeps its number, loads unindexed
// so lookups test its filter and the indexer finishes it, and the next pack
// takes a new number rather than the one the written h rows name.
func TestAddWrittenInterrupted(t *testing.T) {
	// Index pack a, then stop pack b after its first slice.
	ctx := t.Context()
	keysOf := func(id string, n int) [][]byte {
		var keys [][]byte
		for i := range n {
			keys = append(keys, []byte(fmt.Sprintf("block-key-pad-%s-%010d", id, i)))
		}
		return keys
	}
	store := &stopAfterWrites{Store: hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()), writes: 2}
	x := newPackIndex(store)
	if err := x.addWritten(ctx, &packfile.PackfileEntry{Id: "a"}, keysOf("a", 10)); err != nil {
		t.Fatal(err)
	}
	bKeys := keysOf("b", 2*indexBatchKeys+1)
	if err := x.addWritten(ctx, &packfile.PackfileEntry{Id: "b"}, bKeys); err == nil {
		t.Fatal("indexing b did not stop")
	}

	// Reload: b keeps its number unindexed, and its first slice names it.
	store.writes = -1
	got := newPackIndex(store)
	if _, _, err := got.load(ctx); err != nil {
		t.Fatal(err)
	}
	if row := got.rows["b"]; row.indexed || row.num != x.rows["b"].num || got.pending["b"] == nil {
		t.Fatalf("b reloaded as %+v, pending %v", row, got.pending["b"] != nil)
	}
	if id, ok, err := got.Locate(ctx, bKeys[0]); err != nil || !ok || id != "b" {
		t.Fatalf("b's first key located in %q, %v: %v", id, ok, err)
	}
	if _, ok, err := got.Locate(ctx, bKeys[len(bKeys)-1]); err != nil || ok {
		t.Fatalf("b's last key located before its slice: %v", err)
	}

	// The next pack takes a number no h row names.
	if err := got.addWritten(ctx, &packfile.PackfileEntry{Id: "c"}, keysOf("c", 10)); err != nil {
		t.Fatal(err)
	}
	if c, b := got.rows["c"].num, got.rows["b"].num; c == b || !got.rows["c"].indexed {
		t.Fatalf("c took number %d beside b's %d, indexed %v", c, b, got.rows["c"].indexed)
	}
}
