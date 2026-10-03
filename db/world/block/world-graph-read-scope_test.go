package world_block

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestWorldStateLookupGraphQuadsReadOnlyUsesReadOperation(t *testing.T) {
	// Open a background context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Open a testbed.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build a writable mock World.
	writeWs, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer writeWs.Discard()

	// Count read operations on the write store.
	writeStore := &readOperationCountingStore{StoreOps: writeWs.store}
	writeWs.store = writeStore

	// Create objects and set a graph edge.
	{
		createdObject, err := writeWs.CreateObject(ctx, "read-scope/a", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := writeWs.CreateObject(ctx, "read-scope/b", nil)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := writeWs.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("read-scope/a", "<read-scope-rel>", "read-scope/b", "")); err != nil {
		t.Fatal(err.Error())
	}

	// Look up the edge and require no read operation.
	filter := world.NewGraphQuadWithKeys("read-scope/a", "<read-scope-rel>", "", "")
	quads, err := writeWs.LookupGraphQuads(ctx, filter, 10)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 || quads[0].GetObj() != "<read-scope/b>" {
		t.Fatalf("unexpected writable lookup quads: %#v", quads)
	}
	if got := writeStore.beginReadOperations.Load(); got != 0 {
		t.Fatalf("writable lookup opened %d read operations, want 0", got)
	}

	// Commit the write and install its root.
	if err := writeWs.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(writeWs.GetRootRef())
	writeWs.Discard()

	// Build a read-only World on the committed root.
	readWs, err := BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readWs.Discard()

	// Count read operations on the read store.
	readStore := &readOperationCountingStore{StoreOps: readWs.store}
	readWs.store = readStore

	// Look up the edge and require one read operation.
	quads, err = readWs.LookupGraphQuads(ctx, filter, 10)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 || quads[0].GetObj() != "<read-scope/b>" {
		t.Fatalf("unexpected read-only lookup quads: %#v", quads)
	}
	if got := readStore.beginReadOperations.Load(); got != 1 {
		t.Fatalf("read-only lookup opened %d read operations, want 1", got)
	}
}

type readOperationCountingStore struct {
	block.StoreOps

	beginReadOperations atomic.Uint64
}

func (r *readOperationCountingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	r.beginReadOperations.Add(1)
	return r.StoreOps.BeginReadOperation(ctx)
}

// _ is a type assertion
var _ block.StoreOps = (*readOperationCountingStore)(nil)
