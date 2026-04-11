package block_gc

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
)

func newTestRefGraph(t *testing.T) *RefGraph {
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rg.Close() })
	return rg
}

func testBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	ht := hash.HashType_HashType_BLAKE3
	h, err := hash.Sum(ht, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}

func sortedStrings(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func TestAddAndGetOutgoingRefs(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(refs)
	if len(sorted) != 2 || sorted[0] != "b" || sorted[1] != "c" {
		t.Fatalf("expected [b c], got %v", sorted)
	}
}

func TestGetIncomingRefs(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "a", "d"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "b", "d"); err != nil {
		t.Fatal(err)
	}

	sources, err := rg.GetIncomingRefs(ctx, "d")
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(sources)
	if len(sorted) != 2 || sorted[0] != "a" || sorted[1] != "b" {
		t.Fatalf("expected [a b], got %v", sorted)
	}
}

func TestRemoveSingleRef(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}
	if err := rg.RemoveRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "c" {
		t.Fatalf("expected [c], got %v", refs)
	}
}

func TestRemoveNonExistentRef(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Should not error when removing a non-existent edge.
	if err := rg.RemoveRef(ctx, "x", "y"); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveNodeRefs(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "c"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "d"); err != nil {
		t.Fatal(err)
	}

	targets, err := rg.RemoveNodeRefs(ctx, "a", false)
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(targets)
	if len(sorted) != 3 || sorted[0] != "b" || sorted[1] != "c" || sorted[2] != "d" {
		t.Fatalf("expected [b c d], got %v", sorted)
	}

	// Verify all outgoing edges are gone.
	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no outgoing refs, got %v", refs)
	}
}

func TestHasIncomingRefs(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	has, err := rg.HasIncomingRefs(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected b to have incoming refs")
	}

	has, err = rg.HasIncomingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected a to have no incoming refs")
	}
}

func TestHasIncomingRefsExcludesUnreferenced(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Only edge is from "unreferenced" -- should not count.
	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan"); err != nil {
		t.Fatal(err)
	}

	has, err := rg.HasIncomingRefs(ctx, "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected orphan to have no real incoming refs")
	}

	// Add a real ref, now it should count.
	if err := rg.AddRef(ctx, "root", "orphan"); err != nil {
		t.Fatal(err)
	}
	has, err = rg.HasIncomingRefs(ctx, "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected orphan to have incoming refs after adding root edge")
	}
}

func TestHasIncomingRefsExcluding(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, "object:foo", "block:bar"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, NodeUnreferenced, "block:bar"); err != nil {
		t.Fatal(err)
	}

	has, err := rg.HasIncomingRefsExcluding(ctx, "block:bar", "object:foo")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected only excluded and unreferenced edges to be ignored")
	}

	if err := rg.AddRef(ctx, "object:other", "block:bar"); err != nil {
		t.Fatal(err)
	}
	has, err = rg.HasIncomingRefsExcluding(ctx, "block:bar", "object:foo")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected remaining non-excluded incoming ref to be detected")
	}
}

func TestGetUnreferencedNodes(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan1"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, NodeUnreferenced, "orphan2"); err != nil {
		t.Fatal(err)
	}

	nodes, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(nodes)
	if len(sorted) != 2 || sorted[0] != "orphan1" || sorted[1] != "orphan2" {
		t.Fatalf("expected [orphan1 orphan2], got %v", sorted)
	}
}

func TestDiamondDAG(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Build diamond: A -> B, A -> C, B -> D, C -> D
	if err := rg.AddRef(ctx, "A", "B"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "A", "C"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "B", "D"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "C", "D"); err != nil {
		t.Fatal(err)
	}

	// Remove A's outgoing refs (simulating A being collected).
	targets, err := rg.RemoveNodeRefs(ctx, "A", false)
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(targets)
	if len(sorted) != 2 || sorted[0] != "B" || sorted[1] != "C" {
		t.Fatalf("expected [B C], got %v", sorted)
	}

	// B lost its only incoming ref from A.
	has, err := rg.HasIncomingRefs(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected B to have no incoming refs")
	}

	// C lost its only incoming ref from A.
	has, err = rg.HasIncomingRefs(ctx, "C")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected C to have no incoming refs")
	}

	// D still has incoming refs from B and C (edges not yet removed).
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected D to still have incoming refs from B and C")
	}

	// Cascade: remove B's outgoing refs.
	_, err = rg.RemoveNodeRefs(ctx, "B", false)
	if err != nil {
		t.Fatal(err)
	}

	// D still has incoming ref from C.
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected D to still have incoming ref from C")
	}

	// Cascade: remove C's outgoing refs.
	_, err = rg.RemoveNodeRefs(ctx, "C", false)
	if err != nil {
		t.Fatal(err)
	}

	// Now D has no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, "D")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected D to have no incoming refs after full cascade")
	}
}

func TestMultipleRefsFromSameSource(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Adding the same edge twice should be idempotent.
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "a", "b"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "b" {
		t.Fatalf("expected [b], got %v", refs)
	}
}

func TestPermanentRoots(t *testing.T) {
	if !IsPermanentRoot(NodeGCRoot) {
		t.Fatal("gcroot should be permanent")
	}
	if !IsPermanentRoot(NodeUnreferenced) {
		t.Fatal("unreferenced should be permanent")
	}
	if IsPermanentRoot("block:abc") {
		t.Fatal("block:abc should not be permanent")
	}
}

func TestAddBlockRef(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	src := testBlockRef(t, "source-block")
	tgt := testBlockRef(t, "target-block")

	if err := rg.AddBlockRef(ctx, src, tgt); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, BlockIRI(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != BlockIRI(tgt) {
		t.Fatalf("expected [%s], got %v", BlockIRI(tgt), refs)
	}
}

func TestAddBlockRefNil(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	src := testBlockRef(t, "source-block")

	// Nil target should be a no-op.
	if err := rg.AddBlockRef(ctx, src, nil); err != nil {
		t.Fatal(err)
	}
	// Nil source should be a no-op.
	if err := rg.AddBlockRef(ctx, nil, src); err != nil {
		t.Fatal(err)
	}
}

func TestAddObjectRoot(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	ref := testBlockRef(t, "obj-block")
	if err := rg.AddObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != BlockIRI(ref) {
		t.Fatalf("expected [%s], got %v", BlockIRI(ref), refs)
	}
}

func TestRemoveObjectRoot(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	ref := testBlockRef(t, "obj-block")
	if err := rg.AddObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}
	if err := rg.RemoveObjectRoot(ctx, "myobj", ref); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, ObjectIRI("myobj"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %v", refs)
	}
}

func TestDeepCascade(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	// Build a deep chain: gcroot -> E1 -> E2 -> E3 -> E4 -> block
	blk := BlockIRI(testBlockRef(t, "leaf-block"))

	if err := rg.AddRef(ctx, NodeGCRoot, "E1"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E1", "E2"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E2", "E3"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E3", "E4"); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, "E4", blk); err != nil {
		t.Fatal(err)
	}

	// Remove E2 edge from E1 (simulate mid-chain removal).
	if err := rg.RemoveRef(ctx, "E1", "E2"); err != nil {
		t.Fatal(err)
	}

	// E2 should now have no incoming refs.
	has, err := rg.HasIncomingRefs(ctx, "E2")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected E2 to have no incoming refs")
	}

	// Cascade: remove E2's outgoing.
	targets, err := rg.RemoveNodeRefs(ctx, "E2", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != "E3" {
		t.Fatalf("expected [E3], got %v", targets)
	}

	// E3 should have no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, "E3")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected E3 to have no incoming refs")
	}

	// Cascade: remove E3's outgoing.
	targets, err = rg.RemoveNodeRefs(ctx, "E3", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != "E4" {
		t.Fatalf("expected [E4], got %v", targets)
	}

	// Cascade: remove E4's outgoing.
	targets, err = rg.RemoveNodeRefs(ctx, "E4", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != blk {
		t.Fatalf("expected [%s], got %v", blk, targets)
	}

	// Block should have no incoming refs.
	has, err = rg.HasIncomingRefs(ctx, blk)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("expected block to have no incoming refs after cascade")
	}

	// gcroot and E1 should still be connected.
	has, err = rg.HasIncomingRefs(ctx, "E1")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected E1 to still have incoming ref from gcroot")
	}
}

func TestMixedNodeTypes(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	obj := ObjectIRI("world-obj")
	blk1 := BlockIRI(testBlockRef(t, "block1"))
	blk2 := BlockIRI(testBlockRef(t, "block2"))

	// Object references both blocks and an arbitrary entity node.
	if err := rg.AddRef(ctx, obj, blk1); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, obj, blk2); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddRef(ctx, obj, "entity:foo"); err != nil {
		t.Fatal(err)
	}

	refs, err := rg.GetOutgoingRefs(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	sorted := sortedStrings(refs)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 outgoing refs, got %v", sorted)
	}

	// All three should have incoming refs from obj.
	for _, node := range []string{blk1, blk2, "entity:foo"} {
		has, err := rg.HasIncomingRefs(ctx, node)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatalf("expected %s to have incoming refs", node)
		}
	}
}

func TestBlockIRIRoundTrip(t *testing.T) {
	ref := testBlockRef(t, "roundtrip-data")
	iri := BlockIRI(ref)
	if iri == "" {
		t.Fatal("expected non-empty IRI")
	}

	parsed, ok := ParseBlockIRI(iri)
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if !ref.EqualsRef(parsed) {
		t.Fatalf("round-trip mismatch: %s vs %s", ref.MarshalString(), parsed.MarshalString())
	}
}

func TestParseBlockIRIInvalid(t *testing.T) {
	_, ok := ParseBlockIRI("not-a-block-iri")
	if ok {
		t.Fatal("expected parse to fail for non-block IRI")
	}

	_, ok = ParseBlockIRI("block:")
	if ok {
		t.Fatal("expected parse to fail for empty block IRI")
	}

	_, ok = ParseBlockIRI("")
	if ok {
		t.Fatal("expected parse to fail for empty string")
	}
}

func TestBlockIRINilRef(t *testing.T) {
	iri := BlockIRI(nil)
	if iri != "" {
		t.Fatalf("expected empty IRI for nil ref, got %s", iri)
	}

	iri = BlockIRI(&block.BlockRef{})
	if iri != "" {
		t.Fatalf("expected empty IRI for empty ref, got %s", iri)
	}
}

func TestObjectRootNilRef(t *testing.T) {
	ctx := context.Background()
	rg := newTestRefGraph(t)

	if err := rg.AddObjectRoot(ctx, "obj", nil); err != nil {
		t.Fatal(err)
	}
	if err := rg.RemoveObjectRoot(ctx, "obj", nil); err != nil {
		t.Fatal(err)
	}
}

func TestBucketIRI(t *testing.T) {
	iri := BucketIRI("my-bucket-123")
	if iri != "bucket:my-bucket-123" {
		t.Fatalf("expected bucket:my-bucket-123, got %s", iri)
	}
}

func TestParseBucketIRI(t *testing.T) {
	id, ok := ParseBucketIRI("bucket:my-bucket-123")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if id != "my-bucket-123" {
		t.Fatalf("expected my-bucket-123, got %s", id)
	}
}

func TestParseBucketIRIInvalid(t *testing.T) {
	_, ok := ParseBucketIRI("not-a-bucket")
	if ok {
		t.Fatal("expected parse to fail for non-bucket IRI")
	}

	_, ok = ParseBucketIRI("bucket:")
	if ok {
		t.Fatal("expected parse to fail for empty bucket IRI")
	}

	_, ok = ParseBucketIRI("")
	if ok {
		t.Fatal("expected parse to fail for empty string")
	}
}
