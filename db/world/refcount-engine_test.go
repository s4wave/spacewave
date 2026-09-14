package world_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestRefCountEngineRevisionReads checks that revision reads follow committed
// changes through the resolved engine without acquiring caller transactions.
func TestRefCountEngineRevisionReads(t *testing.T) {
	// Instrument only the transaction boundary around the real testbed engine.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	counted := &seqnoCountingEngine{Engine: tb.Engine}
	var inner world.Engine = counted
	engine := world.NewRefCountEngine(ctx, true, func(context.Context, func()) (*world.Engine, func(), error) {
		return &inner, func() {}, nil
	})
	t.Cleanup(engine.ClearContext)
	before, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Commit through a separate handle to the same engine.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, tb.WorldState, "seqno/example", func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("before"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := tb.WorldState.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("seqno/example", "after"), "")
	if err != nil {
		t.Fatal(err)
	}
	if want <= before {
		t.Fatalf("committed revision = %d, want greater than %d", want, before)
	}

	// Both revision reads must use the engine's existing read contract.
	got, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("revision = %d, want %d", got, want)
	}
	if counted.transactions != 0 {
		t.Fatalf("revision reads acquired %d caller transactions", counted.transactions)
	}
	if counted.reads != 2 {
		t.Fatalf("engine revision reads = %d, want 2", counted.reads)
	}
}

// seqnoCountingEngine measures wrapper calls while retaining real engine behavior.
// Its counters are accessed only by the synchronous test caller.
type seqnoCountingEngine struct {
	// Engine supplies the real storage and publication behavior.
	world.Engine
	// transactions counts caller-owned transaction acquisitions.
	transactions int
	// reads counts direct engine revision reads.
	reads int
}

// NewTransaction counts and forwards a caller-owned transaction acquisition.
func (e *seqnoCountingEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	e.transactions++
	return e.Engine.NewTransaction(ctx, write)
}

// GetSeqno counts and forwards a direct revision read.
func (e *seqnoCountingEngine) GetSeqno(ctx context.Context) (uint64, error) {
	e.reads++
	return e.Engine.GetSeqno(ctx)
}

// _ is a type assertion
var _ world.Engine = (*seqnoCountingEngine)(nil)
