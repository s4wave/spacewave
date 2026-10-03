package order

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestGraphPreservesSubtrees proves intrinsic child order survives unordered
// insertion, shared children, a cycle, and a missing external reference.
func TestGraphPreservesSubtrees(t *testing.T) {
	// Create the references used to verify structural ordering.
	root := testRef(t, "root")
	left := testRef(t, "left")
	right := testRef(t, "right")
	leaf := testRef(t, "leaf")
	missing := testRef(t, "missing")

	// Connect ordered children with a shared leaf, cycle, and missing reference.
	graph := NewGraph()
	graph.Add(leaf, []*block.BlockRef{root})
	graph.Add(right, []*block.BlockRef{leaf})
	graph.Add(root, []*block.BlockRef{left, missing, right})
	graph.Add(left, []*block.BlockRef{leaf})

	// Traverse the graph from its root in intrinsic child order.
	refs, err := graph.Order(t.Context(), []*block.BlockRef{root})
	if err != nil {
		t.Fatal(err)
	}

	// Verify shared and cyclic references appear once in subtree order.
	assertRefOrder(t, refs, []*block.BlockRef{root, left, leaf, right})
}

// TestBlockRefsGroupsPartialGraph proves uploads retain available subtrees
// even when the named object root is outside the candidate set.
func TestBlockRefsGroupsPartialGraph(t *testing.T) {
	// Create a partial graph with an object root outside the candidate set.
	parent := testRef(t, "unavailable-parent")
	root := testRef(t, "partial-root")
	child := testRef(t, "partial-child")
	graph := newTestRefGraph()
	graph.add(block_gc.ObjectIRI("object"), block_gc.BlockIRI(parent))
	graph.add(block_gc.BlockIRI(parent), block_gc.BlockIRI(root))
	graph.add(block_gc.BlockIRI(root), block_gc.BlockIRI(child))

	// Order the available blocks through the partial reference graph.
	refs, err := BlockRefs(t.Context(), graph, []*block.BlockRef{child, root})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the available root precedes its child despite the missing parent.
	assertRefOrder(t, refs, []*block.BlockRef{root, child})
}

// TestReplayKeepsStructuralFallback verifies access profiles retain producer
// locality for unprofiled blocks and when the profile identity is stale.
func TestReplayKeepsStructuralFallback(t *testing.T) {
	// Create the references used to verify structural ordering.
	root := testRef(t, "root")
	child := testRef(t, "child")
	hot := testRef(t, "hot")
	refs := []*block.BlockRef{root, child, hot, root}
	record := testAccessOrderRecord(t, []*AccessOrderEntry{{
		Filesystem: AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
		Path:       "entrypoint.mjs", ResolvedRefs: []*block.BlockRef{hot},
	}})
	identity := AccessOrderManifestIdentityFromRecord(record)

	// Replay the current profile over the structural fallback order.
	result, err := ReplayAccessOrderRecordWithFallback(t.Context(), identity, record, refs, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the profiled block precedes the unprofiled subtree.
	assertRefOrder(t, result.Refs, []*block.BlockRef{hot, root, child})

	// Replay a stale profile over the same structural fallback order.
	identity.ManifestRev++
	result, err = ReplayAccessOrderRecordWithFallback(t.Context(), identity, record, refs, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify stale metadata preserves structural order and removes duplicates.
	assertRefOrder(t, result.Refs, []*block.BlockRef{root, child, hot})
}
