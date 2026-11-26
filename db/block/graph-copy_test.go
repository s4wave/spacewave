package block_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
)

// graphTestStore records the refs of each put and serves them with the bytes,
// counting reads and keeping the write order. Blocks put without refs read as
// byte-only.
type graphTestStore struct {
	*block_mock.RefStore

	mu sync.Mutex
	// reads counts GetStoredBlock calls.
	reads int
	// puts lists the refs of written blocks in write order.
	puts []string
	// found holds, for each entry of puts, whether the put found its block.
	found []bool
}

func newGraphTestStore() *graphTestStore {
	return &graphTestStore{RefStore: block_mock.NewRefStore(newOverlayMemoryStore())}
}

func (s *graphTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed := make([]bool, len(entries))
	for i, entry := range entries {
		_, exists, err := s.PutBlock(ctx, entry.Data, &block.PutOpts{ForceBlockRef: entry.Ref, Refs: entry.Refs, RefsKnown: entry.RefsKnown})
		if err != nil {
			return nil, err
		}
		existed[i] = exists
	}
	return existed, nil
}

func (s *graphTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Store the block and its refs, then record the write order.
	ref, existed, err := s.RefStore.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	s.puts = append(s.puts, ref.MarshalString())
	s.found = append(s.found, existed)
	s.mu.Unlock()
	return ref, existed, nil
}

func (s *graphTestStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Count the read, then serve the block with its refs.
	s.mu.Lock()
	s.reads++
	s.mu.Unlock()
	return s.RefStore.GetStoredBlock(ctx, ref)
}

// put writes a block with refs, marking a leaf with an empty list.
func (s *graphTestStore) put(t *testing.T, data string, refs ...*block.BlockRef) *block.BlockRef {
	// Store a test graph block with explicit leaf or child references.
	t.Helper()
	if refs == nil {
		refs = []*block.BlockRef{}
	}
	ref, _, err := s.PutBlock(t.Context(), []byte(data), &block.PutOpts{Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestCopyGraphRebuildsGraphInPostOrder(t *testing.T) {
	// Build a source graph with a shared child and a known subtree.
	ctx := t.Context()
	src := newGraphTestStore()
	shared := src.put(t, "shared")
	left := src.put(t, "left", shared)
	right := src.put(t, "right", shared, shared)
	skipped := src.put(t, "skipped")
	root := src.put(t, "root", left, right, skipped)

	// Copy it, skipping the known subtree and recording completions.
	dst := newGraphTestStore()
	var completed []string
	err := block.CopyGraph(ctx, src, dst, root, &block.GraphCopyOptions{
		Known: func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
			known := make([]bool, len(refs))
			for i, ref := range refs {
				known[i] = ref.EqualsRef(skipped)
			}
			return known, nil
		},
		Complete: func(_ context.Context, ref *block.BlockRef) error {
			completed = append(completed, ref.MarshalString())
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The destination holds every unknown block with its refs, read once.
	for _, ref := range []*block.BlockRef{root, left, right, shared} {
		want, _ := src.GetRefs(ref)
		got, ok := dst.GetRefs(ref)
		if !ok || len(got) != len(want) {
			t.Fatalf("dst refs of %s = %v, want %v", ref.MarshalString(), got, want)
		}
	}
	if _, ok := dst.GetRefs(skipped); ok {
		t.Fatal("known subtree was copied")
	}
	if src.reads != 4 {
		t.Fatalf("source reads = %d, want 4", src.reads)
	}

	// Both writes and completions must reach a child before its parents.
	for name, order := range map[string][]string{"completion": completed, "write": dst.puts} {
		index := func(ref *block.BlockRef) int {
			for i, key := range order {
				if key == ref.MarshalString() {
					return i
				}
			}
			t.Fatalf("%s order misses %s", name, ref.MarshalString())
			return -1
		}
		if len(order) != 4 {
			t.Fatalf("%s order has %d blocks, want 4", name, len(order))
		}
		if index(shared) > index(left) || index(shared) > index(right) ||
			index(left) > index(root) || index(right) > index(root) {
			t.Fatalf("%s order %v is not post-order", name, order)
		}
	}
}

func TestCopyGraphReportsMissingAndUnknownBlocks(t *testing.T) {
	// Create a source graph containing opaque and missing blocks.
	ctx := t.Context()
	src := newGraphTestStore()
	opaque, _, err := src.StoreOps.PutBlock(ctx, []byte("opaque"), nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := src.put(t, "missing")
	if err := src.RmBlock(ctx, missing); err != nil {
		t.Fatal(err)
	}

	// Verify failed subtrees emit no completed block copies.
	var completed int
	opts := &block.GraphCopyOptions{
		Complete: func(context.Context, *block.BlockRef) error {
			completed++
			return nil
		},
	}
	if err := block.CopyGraph(ctx, src, newGraphTestStore(), src.put(t, "a", opaque), opts); !errors.Is(err, block.ErrRefsUnknown) {
		t.Fatalf("byte-only child err = %v, want ErrRefsUnknown", err)
	}
	if err := block.CopyGraph(ctx, src, newGraphTestStore(), src.put(t, "b", missing), opts); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing child err = %v, want ErrNotFound", err)
	}
	if completed != 0 {
		t.Fatalf("completed %d blocks over a failed subtree", completed)
	}
}

func TestCopyGraphLeavesAcceptedHoles(t *testing.T) {
	// Create a root over a missing block and a complete sibling.
	ctx := t.Context()
	src := newGraphTestStore()
	missing := src.put(t, "missing")
	if err := src.RmBlock(ctx, missing); err != nil {
		t.Fatal(err)
	}
	sibling := src.put(t, "sibling")
	holed := src.put(t, "holed", missing)
	root := src.put(t, "root", holed, sibling)

	// Copy past the hole, recording the holes and completed blocks.
	dst := newGraphTestStore()
	var holes []string
	completed := make(map[string]bool)
	err := block.CopyGraph(ctx, src, dst, root, &block.GraphCopyOptions{
		Complete: func(_ context.Context, ref *block.BlockRef) error {
			completed[ref.MarshalString()] = true
			return nil
		},
		Missing: func(_ context.Context, ref *block.BlockRef) error {
			holes = append(holes, ref.MarshalString())
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Every present block is written; only the sibling's subtree is complete.
	if len(holes) != 1 || holes[0] != missing.MarshalString() {
		t.Fatalf("holes = %v, want [%s]", holes, missing.MarshalString())
	}
	if len(dst.puts) != 3 {
		t.Fatalf("wrote %d blocks, want 3", len(dst.puts))
	}
	if !completed[sibling.MarshalString()] || len(completed) != 1 {
		t.Fatalf("completed %v, want only the sibling", completed)
	}
}

// batchCountStore counts the batch reads of a graphTestStore and the most
// refs one batch asked for.
type batchCountStore struct {
	*graphTestStore
	// batches counts GetStoredBlockBatch calls.
	batches int
	// widest is the most refs of one batch.
	widest int
}

func (s *batchCountStore) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	s.mu.Lock()
	s.batches++
	s.widest = max(s.widest, len(refs))
	s.mu.Unlock()
	return block.GetStoredBlockEach(ctx, s.graphTestStore, refs, fn)
}

func TestCopyGraphReadsInBatches(t *testing.T) {
	// Copy and walk graphs of n and 10n blocks, recording allocations per block.
	ctx := t.Context()
	var perBlock []float64
	for _, width := range []int{16, 160} {
		// Build a root over width inner blocks of 64 leaves each.
		src := &batchCountStore{graphTestStore: newGraphTestStore()}
		var inner []*block.BlockRef
		for i := range width {
			var leaves []*block.BlockRef
			for j := range 64 {
				leaves = append(leaves, src.put(t, fmt.Sprintf("leaf %d %d", i, j)))
			}
			inner = append(inner, src.put(t, fmt.Sprintf("inner %d", i), leaves...))
		}
		root := src.put(t, "root", inner...)
		blocks := 1 + width + width*64

		// Copy the graph, then walk it, each over the batching source.
		dst := newGraphTestStore()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if err := block.CopyGraph(ctx, src, dst, root, nil); err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&after)
		perBlock = append(perBlock, float64(after.Mallocs-before.Mallocs)/float64(blocks))
		if len(dst.puts) != blocks || src.reads != blocks {
			t.Fatalf("copy wrote %d and read %d blocks, want %d", len(dst.puts), src.reads, blocks)
		}
		copyBatches := src.batches
		walked := 0
		err := block.WalkGraph(ctx, src, []*block.BlockRef{root}, 0, func(*block.BlockRef, *block.StoredBlock) error {
			walked++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if walked != blocks {
			t.Fatalf("walk visited %d blocks, want %d", walked, blocks)
		}

		// Require each to read in batches of at most an even share of the
		// blocks in flight: about the blocks over the batch size, plus the
		// root and one per inner block while the queue is short.
		for name, batches := range map[string]int{"copy": copyBatches, "walk": src.batches - copyBatches} {
			t.Logf("%d blocks: %s read in %d batches", blocks, name, batches)
			if batches > blocks/(block.DefaultGraphCopyReads/4)+2*width+2 {
				t.Fatalf("%d blocks: %s read in %d batches", blocks, name, batches)
			}
		}
		if src.widest > block.DefaultGraphCopyReads/4 {
			t.Fatalf("a batch asked for %d refs, more than its share of %d", src.widest, block.DefaultGraphCopyReads)
		}
	}

	// Require the allocations per block to stay flat from n to 10n blocks.
	if perBlock[1] > 1.5*perBlock[0] {
		t.Fatalf("copy made %.1f allocations per block at 10n, %.1f at n", perBlock[1], perBlock[0])
	}
}
