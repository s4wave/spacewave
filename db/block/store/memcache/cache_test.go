package block_store_memcache

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// durable is the durable store below the cache, counting its reads. With
// refsUnknown it serves every block with its refs unknown.
type durable struct {
	block.NopStoreOps
	blocks      map[string][]byte
	reads       atomic.Int32
	refsUnknown bool
}

// GetHashType returns the hash type the store builds refs with.
func (d *durable) GetHashType() hash.HashType {
	return hash.RecommendedHashType
}

// GetBlock reads a block.
func (d *durable) GetBlock(_ context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	d.reads.Add(1)
	data, ok := d.blocks[cacheKey(ref)]
	return bytes.Clone(data), ok, nil
}

// GetStoredBlock reads a block with refs known to be none, or unknown when
// refsUnknown is set.
func (d *durable) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, ok, err := d.GetBlock(ctx, ref)
	if err != nil || !ok {
		return nil, err
	}
	return &block.StoredBlock{Data: data, RefsKnown: !d.refsUnknown}, nil
}

// TestCacheServesBoundsAndIsolates reads blocks through a cache: a read fills
// it and the next is served from it, a reader's change to the bytes never
// reaches the cache, existence is never claimed, and blocks past the bound are
// evicted least recently read first.
func TestCacheServesBoundsAndIsolates(t *testing.T) {
	// Ten blocks of 1 KiB below a cache that holds about four.
	ctx := t.Context()
	lower := &durable{blocks: map[string][]byte{}}
	var refs []*block.BlockRef
	for i := range 10 {
		data := bytes.Repeat([]byte{byte(i)}, 1024)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		lower.blocks[cacheKey(ref)] = data
		refs = append(refs, ref)
	}
	c := New(4 * (1024 + entryOverhead))
	reads := c.Reads(func() block.StoreOps { return lower })()
	read := func(i int) []byte {
		t.Helper()
		data, found, err := reads.GetBlock(ctx, refs[i])
		if err != nil || !found || !bytes.Equal(data, lower.blocks[cacheKey(refs[i])]) {
			t.Fatalf("read %d: %v %v", i, found, err)
		}
		return data
	}

	// A read fills the cache, and the next is served from it.
	read(0)
	before := lower.reads.Load()
	data := read(0)
	if lower.reads.Load() != before {
		t.Fatal("a cached block read the durable store")
	}

	// A reader's change to the bytes never reaches the cache.
	data[0] = 0xff
	if again := read(0); again[0] != 0 {
		t.Fatal("a reader changed the cached copy")
	}

	// The cache never claims a block exists.
	if found, err := reads.GetBlockExists(ctx, refs[0]); err != nil || found {
		t.Fatalf("exists = %v, %v", found, err)
	}

	// Past the bound, the least recently read blocks go first.
	for i := 1; i < 5; i++ {
		read(i)
	}
	if st := c.Stats(); st.Blocks != 4 || st.Evicted != 1 || st.Bytes > c.limit {
		t.Fatalf("stats past the bound: %+v", st)
	}
	before = lower.reads.Load()
	read(4)
	if lower.reads.Load() != before {
		t.Fatal("the newest block was evicted")
	}
	read(0)
	if lower.reads.Load() == before {
		t.Fatal("the least recently read block stayed")
	}
}

// TestCacheFillsBlocksWithUnknownRefs reads a block the durable store serves
// with its refs unknown, as most pack blocks are: the read fills the cache,
// the next read is served from it without touching the durable store, and the
// cached block still reports its refs unknown, so no reader takes it for a
// leaf.
func TestCacheFillsBlocksWithUnknownRefs(t *testing.T) {
	// Put a block in a durable store that serves it with unknown refs.
	ctx := t.Context()
	lower := &durable{blocks: map[string][]byte{}, refsUnknown: true}
	data := bytes.Repeat([]byte("unknown refs"), 64)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	lower.blocks[cacheKey(ref)] = data

	// Read the block through the cache.
	c := New(0)
	reads := c.Reads(func() block.StoreOps { return lower })()
	if got, found, err := reads.GetBlock(ctx, ref); err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("first read = %d bytes, %v, %v", len(got), found, err)
	}

	// Read it again: the cache serves it, with its refs still unknown.
	before := lower.reads.Load()
	stored, err := reads.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("second read = %v, %v", stored, err)
	}
	if n := lower.reads.Load() - before; n != 0 {
		t.Fatalf("second read touched the durable store %d times, want 0", n)
	}
	if stored.RefsKnown {
		t.Fatal("the cached block reports its refs known")
	}
	if st := c.Stats(); st.Blocks != 1 || st.Fills != 1 || st.Hits != 1 {
		t.Fatalf("stats = %+v", st)
	}
}
