package world_block

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

func TestWorldState_CreateObjectAppliesRootRefBatch(t *testing.T) {
	ctx := context.Background()
	ws, ocs, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	objKey := "ref-batch/root-object"
	rootRef := writeRefBatchTestBlock(t, ctx, ocs, "root object block")
	rootBlockIRI := block_gc.BlockIRI(rootRef.GetRootRef())
	if rootBlockIRI == "" {
		t.Fatal("expected test root block IRI")
	}

	if err := ws.refGraph.AddRef(ctx, block_gc.NodeUnreferenced, rootBlockIRI); err != nil {
		t.Fatal(err.Error())
	}
	recorder := installRecordingRefGraph(t, ws)

	if _, err := ws.CreateObject(ctx, objKey, rootRef); err != nil {
		t.Fatal(err.Error())
	}

	objIRI := block_gc.ObjectIRI(objKey)
	assertRecordingDirectCalls(t, recorder)
	if len(recorder.applyBatches) != 1 {
		t.Fatalf("ApplyRefBatch calls = %d, want 1", len(recorder.applyBatches))
	}
	assertRefEdgeSet(t, "batch adds", recorder.applyBatches[0].adds, []block_gc.RefEdge{
		{Subject: "world", Object: objIRI},
		{Subject: objIRI, Object: rootBlockIRI},
	})
	assertRefEdgeSet(t, "batch removes", recorder.applyBatches[0].removes, []block_gc.RefEdge{
		{Subject: block_gc.NodeUnreferenced, Object: rootBlockIRI},
	})

	assertOutgoingRefs(t, ctx, recorder, "world", []string{objIRI})
	assertOutgoingRefs(t, ctx, recorder, objIRI, []string{rootBlockIRI})
	assertNotOutgoingRef(t, ctx, recorder, block_gc.NodeUnreferenced, rootBlockIRI)
}

func TestWorldTypes_EnsureTypeExistsAppliesObjectRefBatch(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()
	recorder := installRecordingRefGraph(t, ws)

	typeID := "ref-batch-type"
	created, err := world_types.EnsureTypeExists(ctx, ws, typeID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !created {
		t.Fatal("expected EnsureTypeExists to create the missing type object")
	}

	objKey := world_types.BuildTypeObjectKey(typeID)
	objIRI := block_gc.ObjectIRI(objKey)
	assertRecordingDirectCalls(t, recorder)
	if len(recorder.applyBatches) != 1 {
		t.Fatalf("ApplyRefBatch calls = %d, want 1", len(recorder.applyBatches))
	}
	assertRefEdgeSet(t, "type batch adds", recorder.applyBatches[0].adds, []block_gc.RefEdge{
		{Subject: "world", Object: objIRI},
	})
	assertRefEdgeSet(t, "type batch removes", recorder.applyBatches[0].removes, nil)
	assertOutgoingRefs(t, ctx, recorder, "world", []string{objIRI})

	_, exists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatalf("expected type object %q to exist", objKey)
	}
}

type recordedRefBatch struct {
	adds    []block_gc.RefEdge
	removes []block_gc.RefEdge
}

type recordingRefGraph struct {
	worldRefGraph

	addRefCalls        int
	removeRefCalls     int
	addObjectRootCalls int
	applyBatches       []recordedRefBatch
}

func (r *recordingRefGraph) AddRef(ctx context.Context, subject, object string) error {
	r.addRefCalls++
	return r.worldRefGraph.AddRef(ctx, subject, object)
}

func (r *recordingRefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	r.removeRefCalls++
	return r.worldRefGraph.RemoveRef(ctx, subject, object)
}

func (r *recordingRefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	r.addObjectRootCalls++
	return r.worldRefGraph.AddObjectRoot(ctx, objectKey, ref)
}

func (r *recordingRefGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	r.applyBatches = append(r.applyBatches, recordedRefBatch{
		adds:    cloneRefEdges(adds),
		removes: cloneRefEdges(removes),
	})
	return r.worldRefGraph.ApplyRefBatch(ctx, adds, removes)
}

func newRefBatchTestWorld(t *testing.T, ctx context.Context) (*WorldState, *bucket_lookup.Cursor, func()) {
	t.Helper()

	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	ws, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	if ws.refGraph == nil {
		ws.Discard()
		ocs.Release()
		tb.Release()
		t.Fatal("expected writable world state to have a refgraph")
	}

	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tb.Release()
	}
	return ws, ocs, cleanup
}

func writeRefBatchTestBlock(t *testing.T, ctx context.Context, ocs *bucket_lookup.Cursor, msg string) *bucket.ObjectRef {
	t.Helper()

	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(&block_mock.Example{Msg: msg}, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	return &bucket.ObjectRef{RootRef: rootRef}
}

func installRecordingRefGraph(t *testing.T, ws *WorldState) *recordingRefGraph {
	t.Helper()

	if ws.refGraph == nil {
		t.Fatal("expected refgraph before installing recorder")
	}
	recorder := &recordingRefGraph{worldRefGraph: ws.refGraph}
	ws.refGraph = recorder
	return recorder
}

func assertRecordingDirectCalls(t *testing.T, recorder *recordingRefGraph) {
	t.Helper()

	if recorder.addRefCalls != 0 || recorder.removeRefCalls != 0 || recorder.addObjectRootCalls != 0 {
		t.Fatalf(
			"direct refgraph calls: AddRef=%d RemoveRef=%d AddObjectRoot=%d, want all zero",
			recorder.addRefCalls,
			recorder.removeRefCalls,
			recorder.addObjectRootCalls,
		)
	}
}

func cloneRefEdges(edges []block_gc.RefEdge) []block_gc.RefEdge {
	if len(edges) == 0 {
		return nil
	}
	return slices.Clone(edges)
}

func assertRefEdgeSet(t *testing.T, label string, got, want []block_gc.RefEdge) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	gotCounts := make(map[block_gc.RefEdge]int, len(got))
	for _, edge := range got {
		gotCounts[edge]++
	}
	wantCounts := make(map[block_gc.RefEdge]int, len(want))
	for _, edge := range want {
		wantCounts[edge]++
	}
	for edge, wantCount := range wantCounts {
		if gotCounts[edge] != wantCount {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
	for edge, gotCount := range gotCounts {
		if wantCounts[edge] != gotCount {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
}

func assertOutgoingRefs(t *testing.T, ctx context.Context, rg block_gc.RefGraphOps, node string, want []string) {
	t.Helper()

	got, err := rg.GetOutgoingRefs(ctx, node)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertStringSet(t, "outgoing refs from "+node, got, want)
}

func assertNotOutgoingRef(t *testing.T, ctx context.Context, rg block_gc.RefGraphOps, node, ref string) {
	t.Helper()

	got, err := rg.GetOutgoingRefs(ctx, node)
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(got, ref) {
		t.Fatalf("outgoing refs from %s = %#v, did not want %q", node, got, ref)
	}
}

func assertStringSet(t *testing.T, label string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	gotCounts := make(map[string]int, len(got))
	for _, value := range got {
		gotCounts[value]++
	}
	wantCounts := make(map[string]int, len(want))
	for _, value := range want {
		wantCounts[value]++
	}
	for value, wantCount := range wantCounts {
		if gotCounts[value] != wantCount {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
	for value, gotCount := range gotCounts {
		if wantCounts[value] != gotCount {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
}

var _ worldRefGraph = (*recordingRefGraph)(nil)
