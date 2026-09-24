package order

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestGraphPreservesSubtrees proves intrinsic child order survives unordered
// insertion, shared children, a cycle, and a missing external reference.
func TestGraphPreservesSubtrees(t *testing.T) {
	root := testRef(t, "root")
	left := testRef(t, "left")
	right := testRef(t, "right")
	leaf := testRef(t, "leaf")
	missing := testRef(t, "missing")
	graph := NewGraph()
	graph.Add(leaf, []*block.BlockRef{root})
	graph.Add(right, []*block.BlockRef{leaf})
	graph.Add(root, []*block.BlockRef{left, missing, right})
	graph.Add(left, []*block.BlockRef{leaf})

	refs, err := graph.Order(t.Context(), []*block.BlockRef{root})
	if err != nil {
		t.Fatal(err)
	}
	assertRefOrder(t, refs, []*block.BlockRef{root, left, leaf, right})
}

// TestBlockRefsGroupsPartialGraph proves uploads retain available subtrees
// even when the named object root is outside the candidate set.
func TestBlockRefsGroupsPartialGraph(t *testing.T) {
	parent := testRef(t, "unavailable-parent")
	root := testRef(t, "partial-root")
	child := testRef(t, "partial-child")
	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("object"), block_gc.BlockIRI(parent))
	graph.add(block_gc.BlockIRI(parent), block_gc.BlockIRI(root))
	graph.add(block_gc.BlockIRI(root), block_gc.BlockIRI(child))

	refs, err := BlockRefs(t.Context(), graph, []*block.BlockRef{child, root})
	if err != nil {
		t.Fatal(err)
	}
	assertRefOrder(t, refs, []*block.BlockRef{root, child})
}

// TestReplayKeepsStructuralFallback verifies access profiles retain producer
// locality for unprofiled blocks and when the profile identity is stale.
func TestReplayKeepsStructuralFallback(t *testing.T) {
	root := testRef(t, "root")
	child := testRef(t, "child")
	hot := testRef(t, "hot")
	refs := []*block.BlockRef{root, child, hot, root}
	record := testAccessOrderRecord(t, []*AccessOrderEntry{{
		Filesystem: AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
		Path:       "entrypoint.mjs", ResolvedRefs: []*block.BlockRef{hot},
	}})
	identity := AccessOrderManifestIdentityFromRecord(record)
	result, err := ReplayAccessOrderRecordWithFallback(t.Context(), identity, record, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertRefOrder(t, result.Refs, []*block.BlockRef{hot, root, child})

	identity.ManifestRev++
	result, err = ReplayAccessOrderRecordWithFallback(t.Context(), identity, record, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertRefOrder(t, result.Refs, []*block.BlockRef{root, child, hot})
}
