package world_block

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestDeleteObjectKeepsEndpointRevisions checks that deleting an object leaves
// the revisions of the objects it shared quads with alone: a quad names a
// record without changing it.
func TestDeleteObjectKeepsEndpointRevisions(t *testing.T) {
	// Open an Engine on an empty World.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Release)
	engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	// Commit an attempt sharing two quads with its host, one in each
	// direction, and one with its type.
	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"host", "types/attempt", "host/attempts/1"} {
		obj, err := writer.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []world.GraphQuad{
		world.NewGraphQuadWithKeys("host", "<result>", "host/attempts/1", ""),
		world.NewGraphQuadWithKeys("host/attempts/1", "<host>", "host", ""),
		world.NewGraphQuadWithKeys("host/attempts/1", "<type>", "types/attempt", ""),
	} {
		if err := writer.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Read each endpoint's revision.
	revs := func() map[string]uint64 {
		tx, err := engine.NewBlockEngineTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		out := make(map[string]uint64)
		for _, key := range []string{"host", "types/attempt"} {
			out[key] = objectRev(t, ctx, tx, key)
		}
		return out
	}
	before := revs()

	// Delete the attempt.
	writer, err = engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.DeleteObject(ctx, "host/attempts/1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The host lost two quads and the type one; neither record changed.
	after := revs()
	for key, rev := range before {
		if after[key] != rev {
			t.Fatalf("%s revision %d, want %d", key, after[key], rev)
		}
	}
}

// objectRev returns the revision of the object at key in tx.
func objectRev(t *testing.T, ctx context.Context, tx world.WorldState, key string) uint64 {
	t.Helper()
	obj, found, err := tx.GetObject(ctx, key)
	if err != nil || !found {
		t.Fatal(key, found, err)
	}
	defer world.ReleaseObjectState(obj)
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}
