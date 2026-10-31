package block_gc

// This experiment measures the reads, time and allocations of one block mark
// over a format 4 graph in an on-disk s4db store. It is skipped unless
// SPACEWAVE_GC_MARK_DIR is set. TestMarkReadsBuild writes the graph;
// TestMarkReads opens it in a fresh process and marks it once.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/net/hash"
)

// markReadsEnv returns the integer environment variable name, or def.
func markReadsEnv(t *testing.T, name string, def int) int {
	// Default an unset variable.
	v := os.Getenv(name)
	if v == "" {
		return def
	}

	// Parse a set one.
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		t.Fatalf("%s must be a nonnegative integer", name)
	}
	return n
}

// markReadsPath returns the database path, skipping when not requested.
func markReadsPath(t *testing.T) string {
	dir := os.Getenv("SPACEWAVE_GC_MARK_DIR")
	if dir == "" {
		t.Skip("SPACEWAVE_GC_MARK_DIR is not set")
	}
	return filepath.Join(dir, "markreads.s4db")
}

// markReadsBlock returns the IRI of synthetic block i.
func markReadsBlock(i int) string {
	var seed [8]byte
	binary.LittleEndian.PutUint64(seed[:], uint64(i)) //nolint:gosec // i is nonnegative.
	digest := sha256.Sum256(seed[:])
	return prefixBlock + string([]byte{byte(hash.HashType_HashType_SHA256)}) + string(digest[:])
}

// TestMarkReadsBuild writes a format 4 graph: objects under one bucket, each
// owning a tree of SPACEWAVE_GC_MARK_PER_OBJECT blocks with fanout
// SPACEWAVE_GC_MARK_FANOUT, SPACEWAVE_GC_MARK_CROSS extra child edges per
// thousand blocks to random live blocks, SPACEWAVE_GC_MARK_GARBAGE unowned
// blocks per thousand, and one object owning a chain of
// SPACEWAVE_GC_MARK_CHAIN blocks, each listing the next, like an append-only
// log. Every block has a child list, empty for a leaf.
func TestMarkReadsBuild(t *testing.T) {
	// Read the shape.
	path := markReadsPath(t)
	blocks := markReadsEnv(t, "SPACEWAVE_GC_MARK_BLOCKS", 1_000_000)
	perObject := markReadsEnv(t, "SPACEWAVE_GC_MARK_PER_OBJECT", 142)
	fanout := markReadsEnv(t, "SPACEWAVE_GC_MARK_FANOUT", 6)
	cross := markReadsEnv(t, "SPACEWAVE_GC_MARK_CROSS", 30)
	garbage := blocks * markReadsEnv(t, "SPACEWAVE_GC_MARK_GARBAGE", 20) / 1000
	chain := markReadsEnv(t, "SPACEWAVE_GC_MARK_CHAIN", 0)

	// Prepare the records and the owner edges' writer.
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a reproducible shape.
	total := blocks + garbage
	children := make([][]int32, total+chain)
	type record struct{ key, value []byte }
	var records []record
	addEdge := func(subject, object string) {
		records = append(records,
			record{key: graphKey(kindForward, subject, object)},
			record{key: graphKey(kindIncoming, object, subject)})
	}

	// Collect the owner edges and each tree's children.
	bucket := "bucket:mark-reads"
	addEdge(NodeGCRoot, bucket)
	var edges int
	for base := 0; base < total; base += perObject {
		end := min(base+perObject, total)
		if base < blocks {
			object := ObjectIRI("object/" + strconv.Itoa(base/perObject))
			addEdge(bucket, object)
			addEdge(object, markReadsBlock(base))
		}
		for k := base + 1; k < end; k++ {
			parent := base + (k-base-1)/fanout
			children[parent] = append(children[parent], int32(k)) //nolint:gosec
			edges++
		}
	}

	// Link the chain after the garbage.
	if chain != 0 {
		object := ObjectIRI("object/chain")
		addEdge(bucket, object)
		addEdge(object, markReadsBlock(total))
		for k := total; k < total+chain-1; k++ {
			children[k] = append(children[k], int32(k+1)) //nolint:gosec
			edges++
		}
	}

	// Add the cross edges.
	for k := range blocks {
		for n := cross; n > 0; n -= 1000 {
			if n >= 1000 || rng.IntN(1000) < n {
				children[k] = append(children[k], int32(rng.IntN(blocks))) //nolint:gosec
				edges++
			}
		}
	}

	// Encode the child lists.
	for k := range total + chain {
		kids := make([]string, 0, len(children[k]))
		for _, c := range children[k] {
			kids = append(kids, markReadsBlock(int(c)))
		}
		slices.Sort(kids)
		kids = slices.Compact(kids)
		block := markReadsBlock(k)
		records = append(records, record{key: graphKey(kindChildren, block), value: encodeChildren(kids)})
		children[k] = nil
	}
	slices.SortFunc(records, func(a, b record) int { return bytes.Compare(a.key, b.key) })

	// Open a fresh database.
	_ = os.Remove(path)
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Write the records in key order.
	ctx := context.Background()
	start := time.Now()
	for len(records) != 0 {
		batch := records[:min(len(records), 100_000)]
		records = records[len(batch):]
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range batch {
			if err := tx.Set(ctx, r.key, r.value); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Report the graph.
	st, err := db.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("built %d live blocks, %d garbage, a chain of %d, %d child edges in %s; %d keys, %d run chunks",
		blocks, garbage, chain, edges, time.Since(start).Round(time.Millisecond), st.KeyCount, st.Chunks.Run)
}

// TestMarkReads marks the built graph once and reports its costs.
func TestMarkReads(t *testing.T) {
	// Open the built graph with the chosen cache.
	path := markReadsPath(t)
	if _, err := os.Stat(path); err != nil {
		t.Skip("graph not built")
	}
	cacheMiB := markReadsEnv(t, "SPACEWAVE_GC_MARK_CACHE_MIB", 128)
	db, err := s4db.Open(path, s4db.Options{CacheBytes: cacheMiB << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	// Settle the heap and note its counters.
	g := NewRefGraph(db)
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)

	// Churn when asked: rewrite 64 KiB values outside the graph at the given
	// rate, noting the most freed space a snapshot keeps pending.
	churn := markReadsEnv(t, "SPACEWAVE_GC_MARK_CHURN", 0)
	stop := make(chan struct{})
	churned := make(chan int64)
	go markReadsChurn(t, db, churn, stop, churned)

	// Mark with the chosen mode.
	start := time.Now()
	var mark *BlockMark
	mode := os.Getenv("SPACEWAVE_GC_MARK_MODE")
	switch mode {
	case "scan":
		mark, err = g.MarkBlocksScan(ctx, nil)
	default:
		mark, err = g.MarkBlocks(ctx)
	}
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	// Stop the churn and report its peak.
	close(stop)
	if peak := <-churned; churn != 0 {
		t.Logf("churn %d writes/s: peak pending %d MiB", churn, peak>>20)
	}

	// Read the counters after the mark.
	runtime.ReadMemStats(&m1)
	t.Logf("mode %q scans %d longest read %s", mode, mark.Scans, mark.LongestRead)

	// Check the result against the walk when asked.
	if os.Getenv("SPACEWAVE_GC_MARK_CHECK") != "" {
		want, err := g.MarkBlocks(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(want.Unreached, mark.Unreached) || want.Unknown != mark.Unknown {
			t.Fatalf("unreached %d unknown %d, want %d and %d", len(mark.Unreached), mark.Unknown, len(want.Unreached), want.Unknown)
		}
	}

	// Report the costs.
	t.Logf("mark: live %d unreached %d unknown %d in %s; %d allocs, %d MiB allocated",
		mark.Live.Len(), len(mark.Unreached), mark.Unknown, elapsed.Round(time.Millisecond),
		m1.Mallocs-m0.Mallocs, (m1.TotalAlloc-m0.TotalAlloc)>>20)
}

// markReadsChurn rewrites a 64 KiB value outside the graph rate times a
// second until stop closes, then sends the most pending bytes it saw.
func markReadsChurn(t *testing.T, db *s4db.DB, rate int, stop <-chan struct{}, peak chan<- int64) {
	// Idle when not asked to churn.
	var most int64
	defer func() { peak <- most }()
	if rate == 0 {
		<-stop
		return
	}

	// Rewrite one of 64 keys each tick and sample the pending space.
	ctx := context.Background()
	value := make([]byte, 64<<10)
	tick := time.NewTicker(time.Second / time.Duration(rate))
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Error(err)
			return
		}
		value[0] = byte(i)
		if err := tx.Set(ctx, []byte("zchurn/"+strconv.Itoa(i%64)), value); err != nil {
			t.Error(err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			t.Error(err)
			return
		}
		if st, err := db.Stats(ctx); err == nil {
			most = max(most, st.PendingBytes)
		}
	}
}
