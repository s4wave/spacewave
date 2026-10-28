package block_gc

import (
	"context"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestChildListEncoding checks that a child list round-trips its blocks and
// rejects a node that is not a block.
func TestChildListEncoding(t *testing.T) {
	// Encode sorted blocks of two hash types and decode them back.
	children := []string{BlockIRI(testBlockRef(t, "a")), BlockIRI(testBlockRef(t, "b"))}
	slices.Sort(children)
	got, err := decodeChildren(encodeChildren(children))
	if err != nil || !slices.Equal(got, children) {
		t.Fatalf("decoded %v err=%v, want %v", got, err, children)
	}
	if got, err := decodeChildren(nil); err != nil || len(got) != 0 {
		t.Fatalf("empty list decoded %v err=%v", got, err)
	}

	// A text node is not a child.
	if _, err := decodeChildren(appendNode(nil, "bucket")); err == nil {
		t.Fatal("decoded a text node as a child")
	}
}

// TestMarkBlocksKeepsChildrenOfUnknownBlock checks the failure of October 11:
// an owner reaches a block whose references were never recorded, so the mark
// cannot tell which unreached blocks it names, and the collection sweeps none.
func TestMarkBlocksKeepsChildrenOfUnknownBlock(t *testing.T) {
	// A rooted bucket owns a parent block with no child list. The child it
	// references has a list, so it looks unreached.
	env := newGCTestEnv(t)
	parent, child := env.putBlock(t, "unknown-parent"), env.putBlock(t, "unknown-child")
	rg := env.refGraph
	if err := rg.AddRef(env.ctx, NodeGCRoot, "bucket"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(env.ctx, "bucket", BlockIRI(parent)); err != nil {
		t.Fatal(err)
	}
	deleteChildList(t, env.kvStore, BlockIRI(parent))

	// The mark reports the parent as unknown and the child as unreached.
	mark, err := rg.MarkBlocks(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mark.Unknown != 1 || !slices.Equal(mark.UnknownSample, []string{BlockIRI(parent)}) {
		t.Fatalf("unknown %d %v, want the parent", mark.Unknown, mark.UnknownSample)
	}
	if !slices.Equal(mark.Unreached, []string{BlockIRI(child)}) {
		t.Fatalf("unreached %v, want the child", mark.Unreached)
	}

	// The collection keeps both blocks and reports why.
	stats, err := NewCollector(rg, env.rawStore, nil).Collect(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.UnknownBlocks != 1 || stats.BlocksSwept != 0 {
		t.Fatalf("stats %+v, want one unknown block and no sweep", stats)
	}
	if !env.blockExists(t, parent) || !env.blockExists(t, child) {
		t.Fatal("collection swept past a block of unknown references")
	}

	// A scan mark names the bucket as the record that made the parent live.
	mark, err = rg.markScan(env.ctx, markLimit{records: 1}, nil, nil)
	if err != nil || mark.Unknown != 1 || !slices.Equal(mark.UnknownNamers, []string{"bucket"}) {
		t.Fatalf("scan mark unknown %d namers %v err=%v, want the bucket", mark.Unknown, mark.UnknownNamers, err)
	}

	// A write that records the parent's references between any two read
	// transactions settles it before the mark ends: the mark reaches the
	// child and finds nothing unknown. A mark that ends before the write
	// still reports the parent.
	record := []RefEdge{{Subject: NodeUnreferenced, Object: BlockIRI(parent)}, {Subject: BlockIRI(parent), Object: BlockIRI(child)}}
	for at := 1; ; at++ {
		var calls int
		mark, err = rg.markScan(env.ctx, markLimit{records: 1}, nil, func() error {
			if calls++; calls != at {
				return nil
			}
			return rg.ApplyRefBatch(env.ctx, record, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		if calls < at {
			if mark.Unknown != 1 {
				t.Fatalf("mark without the write: unknown %d, want the parent", mark.Unknown)
			}
			break
		}
		if mark.Unknown != 0 || !mark.Live.Has(BlockIRI(child)) || len(mark.Unreached) != 0 {
			t.Fatalf("write at transaction %d: unknown %d %v, child live %v, unreached %v", at, mark.Unknown, mark.UnknownSample, mark.Live.Has(BlockIRI(child)), mark.Unreached)
		}
		deleteChildList(t, env.kvStore, BlockIRI(parent))
	}

	// Recording the parent's references lets the next pass decide: the
	// parent keeps the child.
	if err := rg.ApplyRefBatch(env.ctx, []RefEdge{{Subject: BlockIRI(parent), Object: BlockIRI(child)}}, nil); err != nil {
		t.Fatal(err)
	}
	mark, err = rg.MarkBlocks(env.ctx)
	if err != nil || mark.Unknown != 0 || len(mark.Unreached) != 0 {
		t.Fatalf("mark after recording: %+v err=%v", mark, err)
	}
}

// TestMarkBlocksMatchesReachability checks the mark against a direct walk on
// random graphs with cycles, shared children, owners of every kind, and child
// lists dropped from reached and unreached blocks.
func TestMarkBlocksMatchesReachability(t *testing.T) {
	for seed := range uint64(40) {
		t.Run(strconv.FormatUint(seed, 10), func(t *testing.T) {
			testMarkBlocksMatchesReachability(t, rand.New(rand.NewPCG(seed, 1)))
		})
	}
}

// testMarkBlocksMatchesReachability runs one graph of
// TestMarkBlocksMatchesReachability.
func testMarkBlocksMatchesReachability(t *testing.T, rnd *rand.Rand) {
	// Open an empty graph.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg := NewRefGraph(store)
	t.Cleanup(func() { rg.Close() })
	blocks := make([]string, 4+rnd.IntN(40))

	// Track every block and link random pairs.
	var edges []RefEdge
	for i := range blocks {
		blocks[i] = BlockIRI(testBlockRef(t, "block-"+strconv.Itoa(i)+"-"+strconv.FormatUint(rnd.Uint64(), 16)))
		edges = append(edges, RefEdge{Subject: NodeUnreferenced, Object: blocks[i]})
	}
	for range rnd.IntN(2 * len(blocks)) {
		edges = append(edges, RefEdge{Subject: blocks[rnd.IntN(len(blocks))], Object: blocks[rnd.IntN(len(blocks))]})
	}

	// Give random owners blocks, and root or queue each owner at random.
	owners := []string{"bucket:a", "bucket:b", "object:c", "stage:d"}
	for range rnd.IntN(4) {
		edges = append(edges, RefEdge{Subject: owners[rnd.IntN(len(owners))], Object: blocks[rnd.IntN(len(blocks))]})
	}
	for _, owner := range owners {
		switch rnd.IntN(3) {
		case 0:
			edges = append(edges, RefEdge{Subject: NodeGCRoot, Object: owner})
		case 1:
			edges = append(edges, RefEdge{Subject: NodeUnreferenced, Object: owner})
		}
	}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Drop the child lists of a few blocks, as an untracked write leaves them.
	untracked := make(map[string]bool)
	for range rnd.IntN(3) {
		node := blocks[rnd.IntN(len(blocks))]
		untracked[node] = true
		deleteChildList(t, store, node)
	}

	// Index the edges the mark follows: every node that is not a block is
	// live, and a block without a child list names nothing.
	next := make(map[string][]string)
	live := make(map[string]bool)
	var stack []string
	for _, edge := range edges {
		if edge.Subject == NodeUnreferenced && slices.Contains(blocks, edge.Object) {
			continue
		}
		if untracked[edge.Subject] {
			continue
		}
		next[edge.Subject] = append(next[edge.Subject], edge.Object)
		for _, node := range []string{edge.Subject, edge.Object} {
			if !slices.Contains(blocks, node) && node != NodeUnreferenced && !live[node] {
				live[node] = true
				stack = append(stack, node)
			}
		}
	}

	// Walk them directly from the live nodes.
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range next[node] {
			if !live[child] {
				live[child] = true
				stack = append(stack, child)
			}
		}
	}

	// Expect the unreached tracked blocks and the reached untracked ones.
	var wantUnreached []string
	var wantUnknown int
	for _, node := range blocks {
		switch {
		case live[node] && untracked[node]:
			wantUnknown++
		case !live[node] && !untracked[node] && !slices.Contains(wantUnreached, node):
			wantUnreached = append(wantUnreached, node)
		}
	}
	slices.Sort(wantUnreached)
	wantUnreached = slices.Compact(wantUnreached)

	// Count the owners of each kind and their distinct forward records. A
	// staging edge to a block keeps no forward record.
	wantOwners := make(map[string]OwnerCount)
	ownerNodes := make(map[string]bool)
	ownerEdges := make(map[RefEdge]bool)
	for _, edge := range edges {
		staged := edge.Subject == NodeUnreferenced && slices.Contains(blocks, edge.Object)
		if staged || slices.Contains(blocks, edge.Subject) || ownerEdges[edge] {
			continue
		}
		ownerEdges[edge] = true
		kind, _, _ := strings.Cut(edge.Subject, ":")
		c := wantOwners[kind]
		c.Edges++
		if !ownerNodes[edge.Subject] {
			ownerNodes[edge.Subject] = true
			c.Nodes++
		}
		wantOwners[kind] = c
	}

	// The walk, the scan in transactions of a few records with a small tail,
	// and the budgeted scan all agree with the direct walk.
	marks := map[string]func() (*BlockMark, error){
		"walk": func() (*BlockMark, error) { return rg.MarkBlocks(ctx) },
		"scan": func() (*BlockMark, error) {
			return rg.markScan(ctx, markLimit{records: 1 + rnd.IntN(3), tail: rnd.IntN(4)}, nil, nil)
		},
		"budgeted scan": func() (*BlockMark, error) { return rg.MarkBlocksScan(ctx, nil) },
	}
	for name, markBlocks := range marks {
		mark, err := markBlocks()
		if err != nil {
			t.Fatal(err)
		}
		unreached := slices.Sorted(slices.Values(mark.Unreached))
		if !slices.Equal(unreached, wantUnreached) || mark.Unknown != wantUnknown {
			t.Fatalf("%s: unreached %d unknown %d, want %d and %d", name, len(unreached), mark.Unknown, len(wantUnreached), wantUnknown)
		}
		if !maps.Equal(mark.Owners, wantOwners) || mark.Roots != 0 {
			t.Fatalf("%s: owners %s and %d roots, want %s and none", name, FormatOwners(mark.Owners), mark.Roots, FormatOwners(wantOwners))
		}
		if len(mark.UnknownSample) != min(wantUnknown, unknownSample) {
			t.Fatalf("%s: sampled %d unknown blocks, want %d", name, len(mark.UnknownSample), wantUnknown)
		}
		for _, node := range blocks {
			if mark.Live.Has(node) != live[node] {
				t.Fatalf("%s: block %q live %v, want %v", name, node, mark.Live.Has(node), live[node])
			}
		}
	}
}

// TestBlockPassRescuesAdoptedBlocks checks that a write during an open pass
// logs the blocks it adopts, that a rescue keeps them and the blocks they
// list, and that writes outside a pass log nothing.
func TestBlockPassRescuesAdoptedBlocks(t *testing.T) {
	// A parent block lists a child, and nothing owns either.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg := NewRefGraph(store)
	t.Cleanup(func() { rg.Close() })
	parent, child := BlockIRI(testBlockRef(t, "adopt-parent")), BlockIRI(testBlockRef(t, "adopt-child"))
	err := rg.ApplyRefBatch(ctx, []RefEdge{
		{Subject: NodeUnreferenced, Object: parent},
		{Subject: NodeUnreferenced, Object: child},
		{Subject: parent, Object: child},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := countKeys(t, store, []byte{kindAdopt}); n != 0 {
		t.Fatalf("writes outside a pass logged %d records", n)
	}

	// Open a pass and mark both blocks unreached.
	if err := rg.BeginBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	mark, err := rg.MarkBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(mark.Unreached) != 2 {
		t.Fatalf("unreached %v, want both blocks", mark.Unreached)
	}

	// A bucket adopts the parent after the snapshot. The rescue keeps the
	// parent and its child and consumes the log.
	if err := rg.AddRef(ctx, "bucket", parent); err != nil {
		t.Fatal(err)
	}
	rescued, _, err := rg.RescueAdopted(ctx, mark, nil)
	if err != nil || rescued != 2 || !mark.Live.Has(parent) || !mark.Live.Has(child) {
		t.Fatalf("rescued %d err=%v", rescued, err)
	}
	if n := countKeys(t, store, []byte{kindAdopt}); n != 0 {
		t.Fatalf("rescue left %d log records", n)
	}

	// Ending the pass drops its marker and any log written since.
	if err := rg.AddRef(ctx, "bucket", child); err != nil {
		t.Fatal(err)
	}
	if err := rg.EndBlockPass(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countKeys(t, store, []byte{kindAdopt}) + countKeys(t, store, passKey); n != 0 {
		t.Fatalf("ended pass left %d records", n)
	}
}

// deleteChildList deletes the child list record of block from store.
func deleteChildList(t *testing.T, store kvtx.Store, block string) {
	// Delete the record in its own commit.
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, graphKey(kindChildren, block)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// countKeys counts the keys of store under prefix.
func countKeys(t *testing.T, store kvtx.Store, prefix []byte) int {
	// Count in a read snapshot.
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	var n int
	if err := tx.ScanPrefixKeys(ctx, prefix, func([]byte) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestNameLaterLogsSettledHoleParent checks that a parent a walk finds naming
// a block the adopt log reached with no namer becomes its namer: settling
// logs it, and once the block is settled the next LogSettled logs it.
func TestNameLaterLogsSettledHoleParent(t *testing.T) {
	// Name blocks for an adopted hole, a parent, a later parent and an owner.
	ref := func(s string) string { return BlockIRI(testBlockRef(t, s)) }
	hole, settled, parent, later := ref("hole"), ref("settled"), ref("parent"), ref("later")

	// The adopt log reached both blocks with no namer.
	mark := &BlockMark{Live: NewNodeSet()}
	adoptedStack(mark, []string{hole, settled})

	// An owner's record names nothing; a parent's list names the hole.
	mark.nameLater(hole, NodeGCRoot)
	mark.nameLater(hole, parent)
	mark.nameLater(hole, later)
	if got := mark.laterNamers[hole]; got != parent {
		t.Fatalf("hole namer %q, want %q", got, parent)
	}

	// Settling the hole logs its later parent.
	mark.noteUnknown(namedNode{node: hole})
	n, err := mark.settleAbsent(func(nodes []string) ([]bool, error) { return make([]bool, len(nodes)), nil })
	if err != nil || n != 1 || !slices.Equal(mark.settledNamers, []string{parent}) {
		t.Fatalf("settled=%d err=%v namers=%v", n, err, mark.settledNamers)
	}

	// A block already settled with no namer logs the parent found after.
	mark.settledNamers = nil
	mark.settled.Add(settled)
	mark.nameLater(settled, later)
	if !slices.Equal(mark.settledNamers, []string{later}) {
		t.Fatalf("namers after settle=%v", mark.settledNamers)
	}
}
