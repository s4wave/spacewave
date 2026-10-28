package world_block

import (
	"testing"

	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestAuditGraphLogFindsLostRecords drops the log records of one quad and the
// node only it names, and checks the audit finds both: the quad whole from
// its postings, the node's value from the object keys.
func TestAuditGraphLogFindsLostRecords(t *testing.T) {
	// Open a World with three objects and two edges.
	ctx := t.Context()
	store := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	ws, err := NewWorldState(ctx, logrus.NewEntry(logrus.New()), true, btx, bcs, store, nil, world_mock.LookupMockOp, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b", "lost"} {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []world.GraphQuad{
		world.NewGraphQuadWithKeys("a", "<next>", "b", ""),
		world.NewGraphQuadWithKeys("b", "<next>", "lost", ""),
	} {
		if err := ws.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.flushGraphWrites(ctx); err != nil {
		t.Fatal(err)
	}

	// Find the node "lost" and the quad naming it in the log.
	tree := ws.graphTree.(*kvtx_block_okra.Tx)
	var records []cayley_proto.Primitive
	err = tree.Salvage(ctx, []byte("l/"), []byte("l0"), func(entry kvtx_block_okra.BuildEntry) bool {
		data, _, err := tree.Get(ctx, entry.Key)
		if err != nil {
			t.Fatal(err)
		}
		var p cayley_proto.Primitive
		if err := p.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		records = append(records, p)
		return true
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lostNode, lostQuad, next uint64
	for _, p := range records {
		if !p.IsNode() {
			continue
		}
		v, err := pquads.UnmarshalValue(ctx, p.Value)
		if err != nil {
			t.Fatal(err)
		}
		switch v {
		case quad.IRI("lost"):
			lostNode = p.ID
		case quad.IRI("next"):
			next = p.ID
		}
	}
	for _, p := range records {
		if !p.IsNode() && p.Object == lostNode {
			lostQuad = p.ID
		}
	}
	if lostNode == 0 || lostQuad == 0 || next == 0 {
		t.Fatalf("log records not found: node=%d quad=%d next=%d", lostNode, lostQuad, next)
	}

	// Drop both log records, as a missing page would.
	for _, id := range []uint64{lostNode, lostQuad} {
		if err := tree.Delete(ctx, graphLogKey(id)); err != nil {
			t.Fatal(err)
		}
	}

	// Audit the log.
	audit, err := ws.AuditGraphLog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.MissingQuads) != 1 || len(audit.MissingNodes) != 1 {
		t.Fatalf("missing quads=%d nodes=%d, want 1 and 1", len(audit.MissingQuads), len(audit.MissingNodes))
	}
	mq := audit.MissingQuads[0]
	if mq.ID != lostQuad || !mq.Complete() || mq.Dirs[1] != next || mq.Dirs[2] != lostNode {
		t.Errorf("missing quad = %+v, want ID %d predicate %d object %d", mq, lostQuad, next, lostNode)
	}
	mn := audit.MissingNodes[0]
	if mn.ID != lostNode || mn.Value != quad.IRI("lost") || mn.Refs != 1 || mn.KnownRefs != 1 {
		t.Errorf("missing node = %+v, want ID %d value <lost> refs 1", mn, lostNode)
	}
	if audit.NodeValues[next] != quad.IRI("next") {
		t.Errorf("predicate value = %v, want <next>", audit.NodeValues[next])
	}
	if audit.Size != 2 || audit.Quads != 1 {
		t.Errorf("size=%d readable quads=%d, want 2 and 1", audit.Size, audit.Quads)
	}
}
