package world_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestPurgeObjectWithoutRecord purges an object whose record block the store
// lacks, which DeleteObject cannot read, and checks its quads go with it.
func TestPurgeObjectWithoutRecord(t *testing.T) {
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

	// Commit a host, a type and an attempt linked to both.
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
		world.NewGraphQuadWithKeys("host/attempts/1", "<type>", "types/attempt", ""),
	} {
		if err := writer.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before, seqno := graphDeleteSnapshot(t, ctx, engine, []string{"host", "types/attempt"})

	// Remove the attempt's record block from the store.
	writer, err = engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	bcs, err := writer.writeTx.state.objTree.GetCursorAtKey(ctx, []byte(objectKeyPrefix+"host/attempts/1"))
	if err != nil || bcs == nil {
		t.Fatalf("record cursor: %v", err)
	}
	if err := tb.Volume.RmBlock(ctx, bcs.GetRef()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writer.GetObject(ctx, "host/attempts/1"); err == nil {
		t.Fatal("reading the removed record succeeded")
	}

	// Purge it and commit.
	purged, err := world.PurgeObject(ctx, writer, "host/attempts/1")
	if err != nil || !purged {
		t.Fatalf("purge = %v, %v", purged, err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The object and its quads are gone and both endpoints were revised.
	after, afterSeqno := graphDeleteSnapshot(t, ctx, engine, []string{"host", "types/attempt"})
	for _, key := range []string{"host", "types/attempt"} {
		if after[key] != before[key]+1 {
			t.Errorf("%s revision = %d, want %d", key, after[key], before[key]+1)
		}
	}
	if afterSeqno <= seqno {
		t.Errorf("seqno = %d, want above %d", afterSeqno, seqno)
	}
	reader, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Discard()
	found, err := reader.HasObject(ctx, "host/attempts/1")
	if err != nil || found {
		t.Fatalf("purged object found=%v err=%v", found, err)
	}
	for _, filter := range []world.GraphQuad{
		world.NewGraphQuadWithKeys("host/attempts/1", "", "", ""),
		world.NewGraphQuadWithKeys("", "", "host/attempts/1", ""),
	} {
		quads, err := reader.LookupGraphQuads(ctx, filter, 0)
		if err != nil || len(quads) != 0 {
			t.Fatalf("quads after purge = %v, %v", quads, err)
		}
	}

	// Purging again finds nothing.
	writer2, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer2.Discard()
	if purged, err := writer2.PurgeObject(ctx, "host/attempts/1"); err != nil || purged {
		t.Fatalf("second purge = %v, %v", purged, err)
	}
}
