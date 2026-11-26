//go:build !tinygo

package block_store_s3

import (
	"context"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// packWalkLatency is the time the fake bucket waits before each GET answers,
// near Backblaze B2's time to first byte.
const packWalkLatency = 50 * time.Millisecond

// TestPackWalkLatency times cold walks of an Okra tree held in packfiles on a
// bucket with B2's GET latency: a batch lookup, a scan, a graph walk and a
// graph copy, each on a store that has read nothing but holds the writer's
// location index, as a Device's does, and single block reads alone and beside
// a walk. It reports each walk's GETs and its rounds, the
// walk's time over one GET's latency.
//
// It uses only APIs master also has, so the same file measures master. It runs
// only when SPACEWAVE_PACK_WALK_KEYS names the tree's key count.
func TestPackWalkLatency(t *testing.T) {
	size, _ := strconv.Atoi(os.Getenv("SPACEWAVE_PACK_WALK_KEYS"))
	if size == 0 {
		t.Skip("set SPACEWAVE_PACK_WALK_KEYS to the tree's key count")
	}
	ctx := t.Context()

	// Build the tree in memory, then write its blocks to packfiles in
	// depth-first order, children in key order, as a copy lays them out.
	src := &packWalkRefs{
		StoreOps: block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), hash.HashType_HashType_BLAKE3),
		refs:     make(map[string][]*block.BlockRef),
	}
	keys, values, refs, rootRef := buildPackWalkTree(t, ctx, src, size)
	bucket, client := newFakeBucket(t, nil)
	index := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	writer := newIndexedStore(t, client, index)
	blocks := writePackWalkTree(t, ctx, src, writer, rootRef)
	waitFor(t, "the index holds every extent", func() bool { return tailsIdle(writer) })
	writer.Close()
	reader := func() *PackStore { return newIndexedStore(t, client, copyPackWalkIndex(t, ctx, index)) }
	t.Logf("%d keys in %d blocks, %d objects", size, blocks, len(bucket.keys()))

	// Count each GET and answer it after the bucket's latency, which holds
	// the connection as a service does.
	var gets atomic.Int64
	bucket.mtx.Lock()
	bucket.before = func(r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("list-type") {
			gets.Add(1)
			time.Sleep(packWalkLatency)
		}
	}
	bucket.mtx.Unlock()

	// measure runs one cold walk on a fresh store and logs its cost.
	measure := func(name string, walk func(store *PackStore)) {
		store := reader()
		gets.Store(0)
		started := time.Now()
		walk(store)
		elapsed := time.Since(started)
		t.Logf("%s: %d GETs, %v, %.1f rounds", name, gets.Load(), elapsed.Round(time.Millisecond), float64(elapsed)/float64(packWalkLatency))
	}
	measure("lookup of every eighth key", func(store *PackStore) {
		var batch [][]byte
		for i := 0; i < size; i += 8 {
			batch = append(batch, keys[i])
		}
		tx := openPackWalkTree(t, ctx, store, rootRef)
		defer tx.Discard()
		got, found, err := tx.GetBatch(ctx, batch)
		if err != nil {
			t.Fatal(err)
		}
		for i := range batch {
			if !found[i] || string(got[i]) != string(values[i*8]) {
				t.Fatalf("lookup of %q returned %q", batch[i], got[i])
			}
		}
	})
	measure("scan", func(store *PackStore) {
		tx := openPackWalkTree(t, ctx, store, rootRef)
		defer tx.Discard()
		it := tx.Iterate(ctx, nil, true, false)
		defer it.Close()
		n := 0
		for it.Next() {
			n++
		}
		if err := it.Err(); err != nil || n != size {
			t.Fatalf("scan returned %d keys: %v", n, err)
		}
	})
	measure("graph walk", func(store *PackStore) {
		n := 0
		err := block.WalkGraph(ctx, store, []*block.BlockRef{rootRef}, 0, func(*block.BlockRef, *block.StoredBlock) error {
			n++
			return nil
		})
		if err != nil || n != blocks {
			t.Fatalf("graph walk visited %d of %d blocks: %v", n, blocks, err)
		}
	})
	measure("graph copy", func(store *PackStore) {
		dst := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), hash.HashType_HashType_BLAKE3)
		if err := block.CopyGraph(ctx, store, dst, rootRef, nil); err != nil {
			t.Fatal(err)
		}
	})

	// Time single reads of random values alone, then beside a graph walk on
	// another store, each reader on a store of its own that has listed the
	// bucket.
	rng := rand.New(rand.NewPCG(1, 2))
	singles := func(store *PackStore, n int) []time.Duration {
		var out []time.Duration
		for range n {
			ref := refs[rng.IntN(size)]
			started := time.Now()
			if stored, err := store.GetStoredBlock(ctx, ref); err != nil || stored == nil {
				t.Fatalf("read %s: %v", ref.MarshalString(), err)
			}
			out = append(out, time.Since(started))
		}
		return out
	}
	alone := reader()
	singles(alone, 1)
	aloneP99 := packWalkP99(singles(alone, 100))

	beside := reader()
	singles(beside, 1)
	walker := reader()
	walkCtx, cancel := context.WithCancel(block.WithBackground(ctx))
	walked := make(chan error, 1)
	go func() {
		walked <- block.WalkGraph(walkCtx, walker, []*block.BlockRef{rootRef}, 0, nil)
	}()
	besideReads := singles(beside, 100)
	cancel()
	<-walked
	besideP99 := packWalkP99(besideReads)
	t.Logf("single reads: p99 %v alone, %v beside a background graph walk (%.0f%%)", aloneP99, besideP99, 100*float64(besideP99)/float64(aloneP99))
}

// buildPackWalkTree writes size values to store and publishes an Okra tree
// mapping each key to its value, returning the keys, the values, their refs
// and the root.
func buildPackWalkTree(t *testing.T, ctx context.Context, store block.StoreOps, size int) ([][]byte, [][]byte, []*block.BlockRef, *block.BlockRef) {
	t.Helper()
	keys := make([][]byte, size)
	values := make([][]byte, size)
	refs := make([]*block.BlockRef, size)
	width := len(strconv.Itoa(size - 1))
	for i := range size {
		digits := strconv.Itoa(i)
		keys[i] = []byte("key-" + strings.Repeat("0", width-len(digits)) + digits)
		values[i] = []byte("value-" + digits)
		ref, _, err := store.PutBlock(ctx, values[i], nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	tx, _, err := kvtx_block_okra.BuildTree(ctx, store, nil, nil, func(yield func([]byte, *block.BlockRef) bool) {
		for i := range keys {
			if !yield(keys[i], refs[i]) {
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return keys, values, refs, rootRef
}

// packWalkRefs keeps the refs each write declares, which the in-memory store
// it wraps does not, and answers each stored block with them.
type packWalkRefs struct {
	block.StoreOps
	// mtx guards refs.
	mtx sync.Mutex
	// refs holds the declared refs of each block written.
	refs map[string][]*block.BlockRef
}

// PutBlock writes a block and keeps its refs.
func (s *packWalkRefs) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, exists, err := s.StoreOps.PutBlock(ctx, data, opts)
	if err == nil {
		s.mtx.Lock()
		s.refs[ref.MapKey()] = opts.GetRefs()
		s.mtx.Unlock()
	}
	return ref, exists, err
}

// PutBlockBatch writes a batch and keeps each block's refs.
func (s *packWalkRefs) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	exists, err := s.StoreOps.PutBlockBatch(ctx, entries)
	if err == nil {
		s.mtx.Lock()
		for _, entry := range entries {
			s.refs[entry.Ref.MapKey()] = entry.Refs
		}
		s.mtx.Unlock()
	}
	return exists, err
}

// GetStoredBlock reads a block with the refs its write declared.
func (s *packWalkRefs) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	s.mtx.Lock()
	refs, known := s.refs[ref.MapKey()]
	s.mtx.Unlock()
	return &block.StoredBlock{Data: data, Refs: refs, RefsKnown: known}, nil
}

// writePackWalkTree writes every block under root from src to dst, depth
// first with each block's children in order, in batches of 1000 blocks, and
// returns the count of blocks.
func writePackWalkTree(t *testing.T, ctx context.Context, src block.StoreOps, dst *PackStore, root *block.BlockRef) int {
	t.Helper()
	var batch []*block.PutBatchEntry
	blocks := 0
	flush := func() {
		if _, err := dst.PutBlockBatch(ctx, batch); err != nil {
			t.Fatal(err)
		}
		batch = batch[:0]
	}
	seen := make(map[string]struct{})
	var visit func(ref *block.BlockRef)
	visit = func(ref *block.BlockRef) {
		key := ref.MapKey()
		if _, ok := seen[key]; ok || ref.GetEmpty() {
			return
		}
		seen[key] = struct{}{}
		stored, err := src.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !stored.RefsKnown {
			t.Fatalf("read %s: %v", ref.MarshalString(), err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true})
		blocks++
		if len(batch) == 1000 {
			flush()
		}
		for _, child := range stored.Refs {
			visit(child)
		}
	}
	visit(root)
	if len(batch) != 0 {
		flush()
	}
	return blocks
}

// copyPackWalkIndex returns a copy of index, for a store of its own.
func copyPackWalkIndex(t *testing.T, ctx context.Context, index kvtx.Store) kvtx.Store {
	t.Helper()
	out := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	rtx, err := index.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	wtx, err := out.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Discard()
	err = rtx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		return wtx.Set(ctx, slices.Clone(key), slices.Clone(value))
	})
	if err == nil {
		err = wtx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// openPackWalkTree opens a read transaction of the tree at root on store.
func openPackWalkTree(t *testing.T, ctx context.Context, store block.StoreOps, root *block.BlockRef) *kvtx_block_okra.Tx {
	t.Helper()
	_, cursor := block.NewTransaction(store, nil, root, nil)
	tx, err := kvtx_block_okra.NewTx(ctx, cursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// packWalkP99 returns the 99th percentile of reads.
func packWalkP99(reads []time.Duration) time.Duration {
	slices.Sort(reads)
	return reads[(len(reads)*99-1)/100]
}
