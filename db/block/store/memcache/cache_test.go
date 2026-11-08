package block_store_memcache

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// durable is the durable store below the cache, counting its reads.
type durable struct {
	block.NopStoreOps
	blocks map[string][]byte
	reads  atomic.Int32
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

// GetStoredBlock reads a block with refs known to be none.
func (d *durable) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, ok, err := d.GetBlock(ctx, ref)
	if err != nil || !ok {
		return nil, err
	}
	return &block.StoredBlock{Data: data, RefsKnown: true}, nil
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
