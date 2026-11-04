package provider_local

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// TestReadSetKeepsRecentReads checks that eviction keeps a read block across
// passes until two generations of readKeepAge have turned.
func TestReadSetKeepsRecentReads(t *testing.T) {
	// Record a read.
	ctx := t.Context()
	now := time.Now()
	r := readSet{clock: func() time.Time { return now }}
	r.enabled.Store(true)
	read, other := hash.NewHash(hash.HashType_HashType_SHA256, []byte("read")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("other"))
	r.add(read)

	// Passes in quick succession keep it.
	take := func() func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return keep
	}
	for range 3 {
		if keep := take(); !keep(read) || keep(other) {
			t.Fatal("pass did not keep exactly the read block")
		}
	}

	// One turn keeps it in the previous generation, the second drops it.
	now = now.Add(readKeepAge)
	if !take()(read) {
		t.Fatal("first turn dropped the read block")
	}
	now = now.Add(readKeepAge)
	if take()(read) {
		t.Fatal("second turn kept the read block")
	}

	// A generation idle for two ages drops both at once.
	r.add(read)
	take()
	now = now.Add(2 * readKeepAge)
	if take()(read) {
		t.Fatal("pass after two idle ages kept the read block")
	}
}

// TestReadSetKeepsReadsAcrossRestart checks that the saved reads keep their
// blocks after a restart, through one turn, and that the second turn drops
// them from the store too.
func TestReadSetKeepsReadsAcrossRestart(t *testing.T) {
	// Open a writeback store to save the reads to.
	ctx := t.Context()
	local := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	wb, err := block_store_writeback.NewStore(ctx, local, local, store_kvtx_inmem.NewStore(), "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	restart := func() *readSet {
		r := &readSet{clock: func() time.Time { return now }}
		r.enabled.Store(true)
		if err := r.load(ctx, wb); err != nil {
			t.Fatal(err)
		}
		return r
	}
	take := func(r *readSet) func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return keep
	}

	// A read saved before a restart keeps its block after it.
	read, later, other := hash.NewHash(hash.HashType_HashType_SHA256, []byte("read")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("later")), hash.NewHash(hash.HashType_HashType_SHA256, []byte("other"))
	r := restart()
	r.add(read)
	if err := r.save(ctx); err != nil {
		t.Fatal(err)
	}
	r = restart()
	if keep := take(r); !keep(read) || keep(other) {
		t.Fatal("restart did not keep exactly the saved read")
	}

	// After one turn and a restart, the previous generation still keeps it,
	// and a read since the turn is kept too.
	now = now.Add(readKeepAge)
	take(r)
	r.add(later)
	if err := r.save(ctx); err != nil {
		t.Fatal(err)
	}
	r = restart()
	if keep := take(r); !keep(read) || !keep(later) {
		t.Fatal("restart after a turn dropped a saved read")
	}

	// The second turn drops the first read, and the store forgets it.
	now = now.Add(readKeepAge)
	if keep := take(r); keep(read) || !keep(later) {
		t.Fatal("second turn did not drop only the first read")
	}
	r = restart()
	if keep := take(r); keep(read) || !keep(later) {
		t.Fatal("store kept the dropped generation")
	}
}

// TestEvictionKeepsCacheHits checks that a block served from a decoded-block
// cache is kept for readKeepAge on the hit set's own clock, without changing
// what the read set keeps: a hit never starts or turns its generations.
func TestEvictionKeepsCacheHits(t *testing.T) {
	// Start a read set and a hit set on one clock.
	ctx := t.Context()
	now := time.Now()
	clock := func() time.Time { return now }
	r := &readSet{clock: clock}
	r.enabled.Store(true)
	hits := &hitSet{clock: clock}
	newRef := func(name string) *block.BlockRef {
		return block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte(name)))
	}
	take := func() func(*hash.Hash) bool {
		keep, err := r.take(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return func(h *hash.Hash) bool { return keep(h) || hits.has(h) }
	}

	// A hit leaves the read set's clock alone: a block read from the store
	// 59 minutes after the hit is still kept by the pass an hour after that,
	// as it was without hits.
	cached, unread, stored := newRef("cached"), newRef("unread"), newRef("stored")
	hitAt := now
	hits.add(cached.GetHash())
	if !r.turned.IsZero() {
		t.Fatal("a cache hit started the read set's generation")
	}
	now = now.Add(59 * time.Minute)
	r.add(stored.GetHash())
	now = now.Add(61 * time.Minute)
	if keep := take(); !keep(stored.GetHash()) || keep(unread.GetHash()) {
		t.Fatal("pass did not keep the stored block alone")
	}

	// The hit is kept for at least readKeepAge on its own clock, and dropped
	// two generations later; a block never hit is not kept.
	now = hitAt.Add(readKeepAge)
	if !hits.has(cached.GetHash()) || hits.has(unread.GetHash()) {
		t.Fatal("hit set dropped the hit within readKeepAge or kept an unread block")
	}
	now = hitAt.Add(2 * readKeepAge)
	if hits.has(cached.GetHash()) {
		t.Fatal("hit set kept the hit past two generations")
	}
}
