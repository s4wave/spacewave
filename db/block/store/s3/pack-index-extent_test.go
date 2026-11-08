//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// countingKV counts the h row writes of its transactions.
type countingKV struct {
	kvtx.Store
	hashSets atomic.Int64
}

func (c *countingKV) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := c.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, kv: c}, nil
}

// countingTx counts the h rows it sets.
type countingTx struct {
	kvtx.Tx
	kv *countingKV
}

func (t *countingTx) Set(ctx context.Context, key, value []byte) error {
	if bytes.HasPrefix(key, []byte(indexHashPrefix)) {
		t.kv.hashSets.Add(1)
	}
	return t.Tx.Set(ctx, key, value)
}

// newIndexedStore opens a sole-writer store over the index kv.
func newIndexedStore(t *testing.T, client *Client, kv kvtx.Store) *PackStore {
	t.Helper()
	s := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, SoleWriter: true})
	t.Cleanup(s.Close)
	return s
}

// tailsIdle reports that no tail awaits the drain.
func tailsIdle(s *PackStore) bool {
	s.index.mtx.RLock()
	defer s.index.mtx.RUnlock()
	return len(s.index.tails.pending) == 0 && !s.index.tails.draining
}

// keysOfRefs returns the block keys of refs.
func keysOfRefs(refs []*block.BlockRef) [][]byte {
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		keys[i] = hashKey(ref)
	}
	return keys
}

// entryOf returns the entry of the count-block pack s holds.
func entryOf(ctx context.Context, t *testing.T, s *PackStore, count uint64) *packfile.PackfileEntry {
	t.Helper()
	if err := s.loadEntries(ctx); err != nil {
		t.Fatal(err)
	}
	var found *packfile.PackfileEntry
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, entry := range s.entries {
			if entry.GetBlockCount() == count {
				found = entry
			}
		}
	})
	if found == nil {
		t.Fatalf("no %d-block pack", count)
	}
	return found
}

// tailOf reads the index tail of a pack from the bucket.
func tailOf(ctx context.Context, t *testing.T, client *Client, entry *packfile.PackfileEntry) []byte {
	t.Helper()
	size := entry.GetSizeBytes()
	tail, err := packfile.ReadIndexTail(ctx, size, size, func(ctx context.Context, off int64, length int) ([]byte, error) {
		return client.GetObjectRange(ctx, "bucket", "p/"+packDir+entry.GetId(), off, length)
	})
	if err != nil {
		t.Fatal(err)
	}
	return tail
}

// checkExtents reads each block through its row's extent alone and checks
// the row names the pack id.
func checkExtents(ctx context.Context, t *testing.T, s *PackStore, client *Client, id string, refs []*block.BlockRef) {
	t.Helper()
	for i, ref := range refs {
		at, off, size, ok, err := s.index.locateExtent(ctx, hashKey(ref))
		if err != nil || !ok {
			t.Fatalf("block %d: no extent: %v", i, err)
		}
		if at != id {
			t.Fatalf("block %d: row names %q, want %q", i, at, id)
		}
		data, err := client.GetObjectRange(ctx, "bucket", "p/"+packDir+at, off, size)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifiedBlock(ref, data); err != nil {
			t.Fatalf("block %d: extent does not hold the block: %v", i, err)
		}
	}
}

// TestIndexExtentWrites writes a pack and adopts one with extent writes on and
// off. On, each block's h row is written once, holding its extent before the
// call returns, and no tail is queued; off, each row is written twice, the
// second time by the tail's drain. Either way every row's extent reads its
// block, so the two paths leave the same rows.
func TestIndexExtentWrites(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run("on="+strconv.FormatBool(on), func(t *testing.T) {
			SetIndexExtentWrites(on)
			t.Cleanup(func() { SetIndexExtentWrites(true) })
			ctx := t.Context()
			_, client := newFakeBucket(t, nil)
			kv := &countingKV{Store: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
			s := newIndexedStore(t, client, kv)

			// The store writes a pack, and adopts one a node uploaded.
			own := adoptBlocks(ctx, t, s, "own", 5)
			node := newTestPackStore(t, client)
			adopted := adoptBlocks(ctx, t, node, "node", 3)
			nodePack := entryOf(ctx, t, node, 3)
			if err := s.AdoptPack(ctx, nodePack, keysOfRefs(adopted)); err != nil {
				t.Fatal(err)
			}
			blocks := int64(len(own) + len(adopted))

			// On, the rows already hold their extents and nothing is queued.
			if on {
				if !tailsIdle(s) {
					t.Fatal("a tail was queued with extent writes on")
				}
				if got := extentRows(t, kv); int64(got) != blocks {
					t.Fatalf("extent rows = %d, want %d", got, blocks)
				}
				if got := kv.hashSets.Load(); got != blocks {
					t.Fatalf("h row writes = %d, want one per block, %d", got, blocks)
				}
			} else {
				waitFor(t, "the tails drain", func() bool { return tailsIdle(s) && int64(extentRows(t, kv)) == blocks })
				if got := kv.hashSets.Load(); got != 2*blocks {
					t.Fatalf("h row writes = %d, want two per block, %d", got, 2*blocks)
				}
			}

			// Either way each row's extent reads its block, in its pack.
			checkExtents(ctx, t, s, client, entryOf(ctx, t, s, 5).GetId(), own)
			checkExtents(ctx, t, s, client, nodePack.GetId(), adopted)
		})
	}
}

// TestExtentValues gives extentValues tails that lie: one without a key,
// bytes that are no tail, a cut tail and the tail of another pack. Each
// yields no extents, so addWritten falls back to rows of the pack alone.
func TestExtentValues(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	s := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, s, "x", 4)
	entry := entryOf(ctx, t, s, 4)
	tail := tailOf(ctx, t, client, entry)
	keys := keysOfRefs(refs)
	other := adoptBlocks(ctx, t, s, "y", 2)
	otherTail := tailOf(ctx, t, client, entryOf(ctx, t, s, 2))

	// The true tail gives every key its extent, naming the pack's number.
	got := extentValues(tail, entry.GetSizeBytes(), 7, keys)
	if len(got) != len(keys) {
		t.Fatalf("extents = %d, want %d", len(got), len(keys))
	}
	for i, v := range got {
		row, ok := parseHashValue(v)
		if !ok || !row.extent || row.num != 7 {
			t.Fatalf("key %d: row %+v %v", i, row, ok)
		}
	}

	// Every lie gives none.
	for name, lie := range map[string]func() [][]byte{
		"extra key": func() [][]byte {
			return extentValues(tail, entry.GetSizeBytes(), 7, append(keys[:4:4], hashKey(other[0])))
		},
		"garbage":      func() [][]byte { return extentValues([]byte("not a packfile index"), entry.GetSizeBytes(), 7, keys) },
		"cut":          func() [][]byte { return extentValues(tail[len(tail)/2:], entry.GetSizeBytes(), 7, keys) },
		"another pack": func() [][]byte { return extentValues(otherTail, entry.GetSizeBytes(), 7, keys) },
		"empty":        func() [][]byte { return extentValues(nil, entry.GetSizeBytes(), 7, keys) },
	} {
		if got := lie(); got != nil {
			t.Fatalf("%s: extents %d, want none", name, len(got))
		}
	}
}

// TestExtentWriteFallsBack adopts a pack through addWritten with a tail that
// lacks one of its keys: the rows name the pack alone, the tail is queued as
// before, and once it drains the keys it lists hold their extents.
func TestExtentWriteFallsBack(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := &countingKV{Store: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	s := newIndexedStore(t, client, kv)
	adoptBlocks(ctx, t, s, "own", 1)

	// A node pack, indexed with an extra key its tail does not list.
	node := newTestPackStore(t, client)
	refs := adoptBlocks(ctx, t, node, "node", 3)
	entry := entryOf(ctx, t, node, 3)
	tail := tailOf(ctx, t, client, entry)
	stray := []byte("a key no pack holds, padded to a block key's length.")
	keys := append(keysOfRefs(refs), packfile.BlockKey(mustRef(t, stray).GetHash()))
	before := kv.hashSets.Load()
	if err := s.index.addWritten(ctx, entry, keys, tail); err != nil {
		t.Fatal(err)
	}

	// The tail drains, upgrading only the rows it lists.
	waitFor(t, "the tail drains", func() bool { return tailsIdle(s) })
	if got := kv.hashSets.Load() - before; got != int64(len(keys)+len(refs)) {
		t.Fatalf("h row writes = %d, want %d inserts and %d upgrades", got, len(keys), len(refs))
	}
	checkExtents(ctx, t, s, client, entry.GetId(), refs)
	if _, _, _, ok, err := s.index.locateExtent(ctx, keys[len(keys)-1]); err != nil || ok {
		t.Fatalf("stray key holds an extent: %v %v", ok, err)
	}
}

// TestExtentRowsNewestPack writes two blocks in one pack, then adopts a pack
// that holds them again: their rows name the newer pack with its extents,
// and the older pack's tail, queued afterwards, does not take them back.
func TestExtentRowsNewestPack(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	s := newIndexedStore(t, client, kv)
	older := adoptBlocks(ctx, t, s, "dup", 2)
	olderPack := entryOf(ctx, t, s, 2)
	checkExtents(ctx, t, s, client, olderPack.GetId(), older)

	// The node's pack holds both blocks again, and a third.
	node := newTestPackStore(t, client)
	newer := adoptBlocks(ctx, t, node, "dup", 3)
	newerPack := entryOf(ctx, t, node, 3)
	if err := s.AdoptPack(ctx, newerPack, keysOfRefs(newer)); err != nil {
		t.Fatal(err)
	}
	checkExtents(ctx, t, s, client, newerPack.GetId(), newer)

	// The older pack's tail drains without moving a row back to it.
	s.index.cacheTail(ctx, olderPack.GetId(), tailOf(ctx, t, client, olderPack))
	waitFor(t, "the older tail drains", func() bool { return tailsIdle(s) })
	checkExtents(ctx, t, s, client, newerPack.GetId(), newer)
}

// TestWrongExtentReadsBlock points a row's extent at the wrong bytes of its
// pack: the read verifies them, rejects them, and finds the block through the
// pack readers, so a wrong extent costs a read and never a wrong block.
func TestWrongExtentReadsBlock(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	s := newIndexedStore(t, client, kv)
	refs := adoptBlocks(ctx, t, s, "w", 3)

	// Move block 0's extent onto block 1's bytes.
	_, off, size, ok, err := s.index.locateExtent(ctx, hashKey(refs[1]))
	if err != nil || !ok {
		t.Fatalf("no extent: %v", err)
	}
	tx, err := kv.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := tx.Get(ctx, hashRowKey(hashKey(refs[0])))
	if err != nil || !found {
		t.Fatalf("row: %v %v", found, err)
	}
	row, _ := parseHashValue(value)
	row.off, row.size = uint64(off), uint32(size) //nolint:gosec // test extents are small.
	if err := tx.Set(ctx, hashRowKey(hashKey(refs[0])), row.marshal()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()

	// The block still reads as itself.
	data, found, err := s.GetBlock(ctx, refs[0])
	if err != nil || !found || string(data) != "w block 0" {
		t.Fatalf("read = %q, %v, %v", data, found, err)
	}
}

func mustRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
