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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	writeWs, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer writeWs.Discard()

	writeStore := &readOperationCountingStore{StoreOps: writeWs.store}
	writeWs.store = writeStore

	if _, err := writeWs.CreateObject(ctx, "read-scope/a", nil); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := writeWs.CreateObject(ctx, "read-scope/b", nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := writeWs.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("read-scope/a", "<read-scope-rel>", "read-scope/b", "")); err != nil {
		t.Fatal(err.Error())
	}

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

	if err := writeWs.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(writeWs.GetRootRef())
	writeWs.Discard()

	readWs, err := BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readWs.Discard()

	readStore := &readOperationCountingStore{StoreOps: readWs.store}
	readWs.store = readStore

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
var _ block.StoreOps = ((*readOperationCountingStore)(nil))
