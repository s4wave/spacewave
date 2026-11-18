package testbed

import (
	"slices"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestRefQuads checks that a record's indexed (world.ref) fields, scalar,
// repeated and nested, become exactly their declared graph quads in the
// creating transaction, and that deleting the quads or the object removes them.
func TestRefQuads(t *testing.T) {
	// Start a World and create the record and every object it references.
	ctx := t.Context()
	tb := MustDefault(t, ctx)
	created := time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)
	rec := &world_mock.MockRefRecord{
		ParentKey: "parent",
		InputKeys: []string{"input-1", "input-2", ""},
		NoteKey:   "note",
		Source:    &world_mock.MockRefSource{ObjectKey: "source"},
		Steps:     []*world_mock.MockRefSource{{ObjectKey: "step-1"}, {}, {ObjectKey: "step-2"}},
		CreatedAt: created.Format(time.RFC3339Nano),
		EndedAt:   timestamp.New(created.Add(time.Hour)),
	}
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, key := range []string{"record", "parent", "input-1", "input-2", "note", "source", "step-1", "step-2"} {
		obj, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := world.SetRefQuads(ctx, tx, "record", rec); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The record has exactly its indexed references, unlabeled; the unindexed
	// note and the empty references have none.
	want := []string{
		"<record> <mock/input> <input-1> ",
		"<record> <mock/input> <input-2> ",
		"<record> <mock/parent> <parent> ",
		"<record> <mock/source> <source> ",
		"<record> <mock/source> <step-1> ",
		"<record> <mock/source> <step-2> ",
	}
	if got := readRecordQuads(t, tb.Engine); !slices.Equal(got, want) {
		t.Fatalf("quads = %q, want %q", got, want)
	}

	// DeleteRefQuads removes the declared quads and keeps the object.
	tx, err = tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := world.DeleteRefQuads(ctx, tx, "record", rec); err != nil {
		t.Fatal(err)
	}
	if quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("record", "", "", ""), 0); err != nil || len(quads) != 0 {
		t.Fatalf("after DeleteRefQuads: quads = %v, err = %v", quads, err)
	}
	if ok, err := tx.HasObject(ctx, "record"); err != nil || !ok {
		t.Fatalf("after DeleteRefQuads: record exists = %v, err = %v", ok, err)
	}
	tx.Discard()

	// Deleting the object removes its reference quads with it.
	tx, err = tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if ok, err := tx.DeleteObject(ctx, "record"); err != nil || !ok {
		t.Fatalf("DeleteObject = %v, %v", ok, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readRecordQuads(t, tb.Engine); len(got) != 0 {
		t.Fatalf("after DeleteObject: quads = %q", got)
	}

	// The declared span reads both time encodings.
	start, end, err := rec.WorldSpan()
	if err != nil || !start.Equal(created) || !end.Equal(created.Add(time.Hour)) {
		t.Fatalf("WorldSpan = %v, %v, %v", start, end, err)
	}
}

// readRecordQuads returns the sorted quads from the object "record", each as
// subject, predicate, object and label joined by spaces.
func readRecordQuads(t *testing.T, eng world.Engine) []string {
	// Read the quads in a read transaction.
	t.Helper()
	ctx := t.Context()
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("record", "", "", ""), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Format and sort them for comparison.
	out := make([]string, 0, len(quads))
	for _, q := range quads {
		out = append(out, q.GetSubject()+" "+q.GetPredicate()+" "+q.GetObj()+" "+q.GetLabel())
	}
	slices.Sort(out)
	return out
}
