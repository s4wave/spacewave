//go:build darwin || linux || windows

package packmove_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

const (
	// worldStoreID is the bucket ID of the fixture's World block store.
	worldStoreID = "p/test/acct/blk/world"
	// otherStoreID is the bucket ID of a store outside the World.
	otherStoreID = "p/test/acct/blk/settings"
	// blockSize is the size of a fixture block's data.
	blockSize = 1024
)

// writebackStoreID returns the object store ID of the World store's records.
func writebackStoreID() string {
	return worldStoreID + "/writeback"
}

// blockData returns the fixture data of the block called name: blockSize bytes
// that differ between names.
func blockData(name string) []byte {
	return bytes.Repeat([]byte(name+"|"), blockSize)[:blockSize]
}

// fixture is a volume file held open by this process, with a gc-tracked
// block store for the World and one for another bucket.
type fixture struct {
	// t is the test.
	t *testing.T
	// ctx is the test context.
	ctx context.Context
	// db is the volume file.
	db *s4db.DB
	// vol is the volume over db; closing it closes db.
	vol *volume_kvtx.Volume
	// keys is the key layout.
	keys *store_kvkey.KVKey
	// world writes blocks owned by the World's bucket.
	world *block_gc.GCStoreOps
	// other writes blocks owned by another bucket.
	other *block_gc.GCStoreOps
}

// openFixture opens the volume file at path for writing, creating it, with the
// World and another bucket rooted in the reference graph.
func openFixture(t *testing.T, path string, opts s4db.Options) *fixture {
	// Open the file with a volume over it.
	t.Helper()
	ctx := t.Context()
	keys, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	db, err := s4db.Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Build the volume, closing the file with it.
	vol, err := volume_kvtx.NewVolume(ctx, "packmove-test", keys, db, &store_kvtx.Config{}, false, false, nil, db.Close)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: ctx, db: db, vol: vol, keys: keys}

	// Root both buckets and build their stores.
	rg := vol.GetRefGraph()
	for _, id := range []string{worldStoreID, otherStoreID} {
		if err := rg.AddRef(ctx, block_gc.NodeGCRoot, block_gc.BucketIRI(id)); err != nil {
			t.Fatal(err)
		}
	}
	f.world = block_gc.NewGCStoreOpsWithParent(vol, rg, block_gc.BucketIRI(worldStoreID))
	f.other = block_gc.NewGCStoreOpsWithParent(vol, rg, block_gc.BucketIRI(otherStoreID))
	return f
}

// put writes the block called name to store with the refs, flushes its graph
// records and returns its ref.
func (f *fixture) put(store *block_gc.GCStoreOps, name string, refs ...*block.BlockRef) *block.BlockRef {
	// Write the block and flush its graph records.
	f.t.Helper()
	ref, _, err := store.PutBlock(f.ctx, blockData(name), &block.PutOpts{RefsKnown: true, Refs: refs})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := store.FlushPending(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	return ref
}

// putWorld writes count World blocks named prefix-0 to prefix-(count-1), each
// referencing the one before it, and returns their refs.
func (f *fixture) putWorld(prefix string, count int) []*block.BlockRef {
	var refs []*block.BlockRef
	for i := range count {
		var children []*block.BlockRef
		if i != 0 {
			children = append(children, refs[i-1])
		}
		refs = append(refs, f.put(f.world, prefix+"-"+strconv.Itoa(i), children...))
	}
	return refs
}

// putRaw writes the key and value straight to the volume file.
func (f *fixture) putRaw(key, value []byte) {
	// Commit the key in its own transaction.
	f.t.Helper()
	tx, err := f.db.NewTransaction(f.ctx, true)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := tx.Set(f.ctx, key, value); err != nil {
		tx.Discard()
		f.t.Fatal(err)
	}
	if err := tx.Commit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

// putBad stores data that does not hash to ref under ref's block key.
func (f *fixture) putBad(name string) *block.BlockRef {
	// Build the ref of the real data, then store other data under it.
	f.t.Helper()
	ref, err := block.BuildBlockRef(blockData(name), nil)
	if err != nil {
		f.t.Fatal(err)
	}
	key, err := ref.MarshalKey()
	if err != nil {
		f.t.Fatal(err)
	}
	f.putRaw(f.keys.GetBlockKey(key), []byte("rotted"))
	return ref
}

// putOldRecord stores a write-back upload record of the source volume, which
// the switch must not carry across.
func (f *fixture) putOldRecord(ref *block.BlockRef) {
	// Write the marker and queue records under the write-back prefix.
	f.t.Helper()
	marked, queue := block_writeback.MarkerKeys(block_writeback.Mark{Hash: ref.GetHash(), Size: blockSize})
	prefix := f.keys.GetObjectStorePrefixByID(writebackStoreID())
	f.putRaw(append(bytes.Clone(prefix), marked...), nil)
	f.putRaw(append(bytes.Clone(prefix), queue...), []byte(strconv.Itoa(blockSize)))
}

// close closes the volume and the file under it.
func (f *fixture) close() {
	f.t.Helper()
	if _, err := f.vol.Sync(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	if err := f.vol.Close(); err != nil {
		f.t.Fatal(err)
	}
}

// newKeys returns the default key layout.
func newKeys(t *testing.T) *store_kvkey.KVKey {
	t.Helper()
	keys, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// itoa formats n.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// countTrue counts the true values of m.
func countTrue(m map[string]bool) int {
	var n int
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}
