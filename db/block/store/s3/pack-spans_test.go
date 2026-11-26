//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

// putTree writes a tree of the given depth, each structure block with fanout
// children, as one batch, and returns its root and its block count.
func putTree(ctx context.Context, t *testing.T, s *PackStore, depth, fanout int) (*block.BlockRef, int) {
	t.Helper()
	var batch []*block.PutBatchEntry
	var build func(level, at int) *block.BlockRef
	build = func(level, at int) *block.BlockRef {
		// Build the children first, so the node can name them.
		var refs []*block.BlockRef
		if level < depth-1 {
			for i := range fanout {
				refs = append(refs, build(level+1, at*fanout+i))
			}
		}
		data := []byte(fmt.Sprintf("node %d.%d with %d children", level, at, len(refs)))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		batch = append(batch, &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true})
		return ref
	}
	root := build(0, 0)
	if _, err := s.PutBlockBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	return root, len(batch)
}

// walkStructure walks the structure blocks of the tree of depth under root on
// a cold store over kv, skipping the leaves, and returns the blocks it
// visited, the range reads it made, the store's read counts and its
// allocations per block.
func walkStructure(ctx context.Context, t *testing.T, client *Client, kv kvtx.Store, root *block.BlockRef, depth int) (int, int64, ReadCounts, float64) {
	t.Helper()
	objs := &countingObjects{ObjectStore: client}
	s := NewPackStore(logrus.NewEntry(logrus.New()), objs, "bucket", "p/", &PackStoreOptions{KeepPacks: true, Index: kv, RefreshInterval: time.Hour})
	t.Cleanup(s.Close)
	if err := s.Preload(ctx); err != nil {
		t.Fatal(err)
	}
	lastLevel := fmt.Sprintf("node %d.", depth-2)
	visited := 0
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := block.WalkGraph(ctx, s, []*block.BlockRef{root}, 16, func(_ *block.BlockRef, stored *block.StoredBlock) error {
		if stored == nil {
			return fmt.Errorf("block %d missing", visited)
		}
		visited++
		if strings.HasPrefix(string(stored.Data), lastLevel) {
			return block.ErrSkipRefs
		}
		return nil
	})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	return visited, objs.rangeReads.Load(), s.ReadCounts(), float64(after.Mallocs-before.Mallocs) / float64(visited)
}

// TestReachReadScales walks the structure of a tree of fanout 10, 111 and then
// 1111 structure blocks over 10 times as many leaves, on a cold store, after a
// first round that warms the process. Laid out in graph order, the walk reads
// the whole structure with the root's reach, one range read at either size,
// and cuts every deeper level from the held span, with allocations per block
// flat. Laid out in key order, the same walk reads once per batch.
func TestReachReadScales(t *testing.T) {
	ctx := t.Context()
	was := packGraphOrder.Load()
	t.Cleanup(func() { SetPackGraphOrder(was) })

	var perBlock []float64
	for _, depth := range []int{4, 4, 5} {
		var reads [2]int64
		for i, graph := range []bool{false, true} {
			// Write the tree as one pack and wait for its extents.
			SetPackGraphOrder(graph)
			_, client := newFakeBucket(t, nil)
			kv := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
			writer := newIndexedStore(t, client, kv)
			root, _ := putTree(ctx, t, writer, depth, 10)
			waitFor(t, "the index holds every extent", func() bool { return tailsIdle(writer) })
			writer.Close()

			// Walk it on a cold store.
			visited, got, counts, allocs := walkStructure(ctx, t, client, kv, root, depth)
			if want := structureBlocks(depth, 10); visited != want {
				t.Fatalf("depth %d: visited %d of %d structure blocks", depth, visited, want)
			}
			reads[i] = got
			if graph {
				if counts.Spanned == 0 {
					t.Fatalf("depth %d: no run was cut from a held span", depth)
				}
				perBlock = append(perBlock, allocs)
			}
		}
		t.Logf("depth %d: %d range reads in key order, %d in graph order", depth, reads[0], reads[1])
		if reads[1] != 1 {
			t.Fatalf("depth %d: graph order walk made %d range reads, want 1", depth, reads[1])
		}
		if reads[0] <= reads[1] {
			t.Fatalf("depth %d: key order walk made %d range reads, no more than graph order's %d", depth, reads[0], reads[1])
		}
	}
	t.Logf("allocations per block: %.1f at n, %.1f at 10n", perBlock[1], perBlock[2])
	if perBlock[2] > 1.5*perBlock[1] {
		t.Fatalf("walk made %.1f allocations per block at 10n, %.1f at n", perBlock[2], perBlock[1])
	}
}

// structureBlocks returns the structure blocks of a tree of depth with fanout
// children under each.
func structureBlocks(depth, fanout int) int {
	count, level := 0, 1
	for range depth - 1 {
		count += level
		level *= fanout
	}
	return count
}

// TestSpanBufferHoldsTrueBytes adds random, overlapping spans of three packs
// whose byte at each offset is known, to a buffer small enough to drop most
// of them, and checks that every read it answers returns the pack's own bytes,
// that the newest span still answers, and that the spans stay within the
// limit and none lies inside another.
func TestSpanBufferHoldsTrueBytes(t *testing.T) {
	packByte := func(id string, off int64) byte { return byte(int64(id[0])*31 + off*7) }
	span := func(id string, off int64, n int) []byte {
		data := make([]byte, n)
		for i := range data {
			data[i] = packByte(id, off+int64(i))
		}
		return data
	}
	rng := rand.New(rand.NewPCG(1, 2))
	ids := []string{"a", "b", "c"}
	b := newSpanBuffer(64 << 10)
	for range 20000 {
		id := ids[rng.IntN(len(ids))]
		off, n := rng.Int64N(1<<16), 1+rng.IntN(4096)
		b.add(id, off, span(id, off, n))
		if got, ok := b.read(id, off, n); !ok || !bytes.Equal(got, span(id, off, n)) {
			t.Fatalf("newest span %s@%d+%d did not read back", id, off, n)
		}

		// Read a random range, which must be the pack's bytes when answered.
		rid := ids[rng.IntN(len(ids))]
		roff, rn := rng.Int64N(1<<16), 1+rng.IntN(512)
		if got, ok := b.read(rid, roff, rn); ok && !bytes.Equal(got, span(rid, roff, rn)) {
			t.Fatalf("read %s@%d+%d returned another pack's or offset's bytes", rid, roff, rn)
		}
	}

	// Check the charge and that no span lies inside another of its pack.
	held := 0
	var prev *heldSpan
	b.spans.Scan(func(sp *heldSpan) bool {
		held += sp.cost()
		if prev != nil && prev.id == sp.id && prev.end() >= sp.end() {
			t.Errorf("span %s@%d lies inside %s@%d", sp.id, sp.off, prev.id, prev.off)
		}
		prev = sp
		return true
	})
	if held != b.held || held > b.limit || b.order.Len() != b.spans.Len() {
		t.Fatalf("held %d, counted %d, limit %d, %d in order, %d in tree", held, b.held, b.limit, b.order.Len(), b.spans.Len())
	}
}
