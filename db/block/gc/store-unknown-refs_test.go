package block_gc

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestGCStoreOps_ParentIRI_ReputWithoutRefsKeepsChildren tests that writing a
// block stored without tracking again, without declaring its references, does
// not record it as a leaf: the sweep keeps the child it references.
func TestGCStoreOps_ParentIRI_ReputWithoutRefsKeepsChildren(t *testing.T) {
	// Root the bucket.
	parent := BucketIRI("undeclared-bucket")
	env := newGCTestEnvWithParent(t, parent)
	if err := env.refGraph.AddRef(env.ctx, NodeGCRoot, parent); err != nil {
		t.Fatal(err.Error())
	}

	// An untracked writer stores a directory and its child.
	leaf := buildTreeEntry(t, "undeclared-leaf")
	dir := buildTreeEntry(t, "undeclared-dir", leaf)
	if _, err := env.rawStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{leaf, dir}); err != nil {
		t.Fatal(err.Error())
	}

	// The bucket stores the child, so the bucket holds it.
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{leaf}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// The bucket stores the directory again without declaring its references,
	// then releases the child it held directly.
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{{Ref: dir.Ref, Data: dir.Data}}); err != nil {
		t.Fatal(err.Error())
	}
	release := []RefEdge{{Subject: parent, Object: BlockIRI(leaf.Ref)}}
	env.flush(t)
	if err := env.refGraph.ApplyRefBatch(env.ctx, nil, release); err != nil {
		t.Fatal(err.Error())
	}

	// The sweep keeps the child the directory references.
	collector := NewCollector(env.refGraph, env.rawStore, nil)
	for range 2 {
		if _, err := collector.Collect(env.ctx); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, entry := range []*block.PutBatchEntry{leaf, dir} {
		if !env.blockExists(t, entry.Ref) {
			t.Fatalf("reachable block %q was swept", entry.Data)
		}
	}
}

// TestGCStoreOps_ParentIRI_TakenUntrackedBlockReadsUnknown tests that a block
// stored without tracking and then taken by a parent without declared
// references reads with unknown references, not as a leaf a copy would record,
// and that a take declaring its references known records them, a leaf's
// empty list included.
func TestGCStoreOps_ParentIRI_TakenUntrackedBlockReadsUnknown(t *testing.T) {
	// An untracked writer stores a directory and its child.
	env := newGCTestEnvWithParent(t, BucketIRI("taken-bucket"))
	leaf := buildTreeEntry(t, "taken-leaf")
	dir := buildTreeEntry(t, "taken-dir", leaf)
	if _, err := env.rawStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{leaf, dir}); err != nil {
		t.Fatal(err.Error())
	}

	// The bucket stores the directory again without declaring its references.
	if _, err := env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{{Ref: dir.Ref, Data: dir.Data}}); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// The read reports unknown references rather than an empty list.
	stored, err := env.gcStore.GetStoredBlock(env.ctx, dir.Ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stored == nil {
		t.Fatal("taken block not found")
	}
	if stored.RefsKnown {
		t.Fatalf("taken untracked block reads as known with refs %v", stored.Refs)
	}

	// The bucket takes both again, declaring their references known.
	known := []*block.PutBatchEntry{
		{Ref: leaf.Ref, Data: leaf.Data, RefsKnown: true},
		{Ref: dir.Ref, Data: dir.Data, Refs: dir.Refs, RefsKnown: true},
	}
	if _, err := env.gcStore.PutBlockBatch(env.ctx, known); err != nil {
		t.Fatal(err.Error())
	}
	env.flush(t)

	// Each reads as known: the leaf with no references, the directory with
	// its child.
	for _, entry := range []*block.PutBatchEntry{leaf, dir} {
		stored, err := env.gcStore.GetStoredBlock(env.ctx, entry.Ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !stored.GetRefsKnown() || len(stored.Refs) != len(entry.Refs) {
			t.Fatalf("block %q reads known=%v refs=%v after a known take", entry.Data, stored.GetRefsKnown(), stored.Refs)
		}
	}
}
