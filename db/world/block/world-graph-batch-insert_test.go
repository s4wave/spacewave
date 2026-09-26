package world_block

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/world"
)

// TestGraphInsertionBatchMatchesIndividualWrites verifies graph contents,
// endpoint revisions, duplicate handling, and retained World change records.
func TestGraphInsertionBatchMatchesIndividualWrites(t *testing.T) {
	ctx := t.Context()
	var wantRevisions []uint64
	var wantChanges []*WorldChange
	for _, batch := range []bool{false, true} {
		ws, cursor, cleanup := newRefBatchTestWorld(t, ctx)
		defer cleanup()
		ref := writeRefBatchTestBlock(t, ctx, cursor, "body")
		keys := []string{"from", "to"}
		for _, key := range keys {
			object, err := ws.CreateObject(ctx, key, ref)
			world.ReleaseObjectState(object)
			if err != nil {
				t.Fatal(err)
			}
		}
		quads := make([]world.GraphQuad, 256)
		for i := range quads {
			quads[i] = world.NewGraphQuadWithKeys("from", "<edge>", "to", fmt.Sprintf("<label-%03d>", i))
		}
		quads = append(quads, world.NewGraphQuadWithKeys("from", "<edge>", "from", ""))
		start := time.Now()
		if batch {
			if err := ws.InsertGraphQuads(ctx, quads); err != nil {
				t.Fatal(err)
			}
		} else {
			for _, q := range quads {
				if err := ws.SetGraphQuad(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Logf("batch=%v insertion=%s", batch, time.Since(start))
		found, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
		if err != nil || len(found) != len(quads) {
			t.Fatalf("graph contains %d quads, want %d: %v", len(found), len(quads), err)
		}
		for _, q := range quads {
			found, err := ws.LookupGraphQuads(ctx, q, 1)
			if err != nil || len(found) != 1 {
				t.Fatalf("missing quad %v: %v", q, err)
			}
		}
		before := len(ws.pendingChanges)
		if err := ws.SetGraphQuad(ctx, quads[0]); err != nil {
			t.Fatal(err)
		}
		if len(ws.pendingChanges) != before {
			t.Fatal("duplicate changed the World")
		}
		refs, err := ws.GetObjectRootRefsBatch(ctx, keys)
		if err != nil {
			t.Fatal(err)
		}
		revisions := []uint64{refs[0].Rev, refs[1].Rev}
		if err := ws.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		entries, err := ReadChangeLogEntriesFromCursor(ctx, ws.bcs, ChangeLogReadOptions{Limit: 1})
		if err != nil || len(entries) != 1 {
			t.Fatalf("read retained changes: count=%d err=%v", len(entries), err)
		}
		var changes []*WorldChange
		for _, change := range entries[0].Changes {
			if change.GetChangeType() == WorldChangeType_WorldChange_GRAPH_SET {
				changes = append(changes, change)
			}
		}
		if len(changes) != len(quads) {
			t.Fatalf("recorded %d graph changes, want %d", len(changes), len(quads))
		}
		if !batch {
			wantRevisions, wantChanges = revisions, changes
		} else if !reflect.DeepEqual(revisions, wantRevisions) || !reflect.DeepEqual(changes, wantChanges) {
			t.Fatal("batch differs from individual endpoint revisions or World changes")
		}
	}
}

// TestGraphInsertionValidatesBatch rejects missing endpoints before insertion
// and rejects repeated relationships without changing SetGraphQuad idempotence.
func TestGraphInsertionValidatesBatch(t *testing.T) {
	ctx := t.Context()
	ws, cursor, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()
	ref := writeRefBatchTestBlock(t, ctx, cursor, "body")
	object, err := ws.CreateObject(ctx, "from", ref)
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	valid := world.NewGraphQuadWithKeys("from", "<edge>", "from", "")
	invalid := world.NewGraphQuadWithKeys("from", "<edge>", "missing", "")
	if err := ws.InsertGraphQuads(ctx, []world.GraphQuad{valid, invalid}); err == nil {
		t.Fatal("accepted missing endpoint")
	}
	found, err := ws.LookupGraphQuads(ctx, valid, 0)
	if err != nil || len(found) != 0 {
		t.Fatalf("inserted a partial batch: %v, %v", found, err)
	}
	if err := ws.InsertGraphQuads(ctx, []world.GraphQuad{valid, valid}); err == nil {
		t.Fatal("accepted repeated relationship")
	}
}

// TestGraphInsertionImportSpansBatches verifies that a fresh graph import
// larger than one Cayley batch reuses nodes across batches and keeps every
// relationship.
func TestGraphInsertionImportSpansBatches(t *testing.T) {
	ctx := t.Context()
	ws, cursor, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()
	if _, packed := ws.graphTree.(*kvtx_block_okra.Tx); !packed {
		t.Fatal("test World graph index is not packed")
	}
	ref := writeRefBatchTestBlock(t, ctx, cursor, "body")
	for _, key := range []string{"from", "to"} {
		object, err := ws.CreateObject(ctx, key, ref)
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
	}
	quads := make([]world.GraphQuad, graphImportBatchSize+1)
	for i := range quads {
		quads[i] = world.NewGraphQuadWithKeys("from", "<edge>", "to", fmt.Sprintf("<label-%05d>", i))
	}
	if err := ws.InsertGraphQuads(ctx, quads); err != nil {
		t.Fatal(err)
	}

	found, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
	if err != nil || len(found) != len(quads) {
		t.Fatalf("graph contains %d quads, want %d: %v", len(found), len(quads), err)
	}
	for _, q := range []world.GraphQuad{quads[0], quads[len(quads)-1]} {
		found, err := ws.LookupGraphQuads(ctx, q, 1)
		if err != nil || len(found) != 1 {
			t.Fatalf("missing quad %v: %v", q, err)
		}
	}
}
