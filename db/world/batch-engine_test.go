package world_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestBatchEngineAtomicClients verifies read-your-writes, one publication and
// rollback when a nested client discards a write but its caller ignores failure.
func TestBatchEngineAtomicClients(t *testing.T) {
	tb := world_testbed.MustDefault(t, t.Context())
	counted := &batchCountEngine{Engine: tb.Engine}
	engine := world.NewBatchEngine(counted)
	create := func(ctx context.Context, key string) error {
		return world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
			object, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(object)
			return err
		})
	}
	var escaped context.Context
	err := engine.Run(t.Context(), func(ctx context.Context) error {
		escaped = ctx
		if err := create(ctx, "first"); err != nil {
			return err
		}
		return world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
			object, err := world.MustGetObject(ctx, ws, "first")
			world.ReleaseObjectState(object)
			if err != nil {
				return err
			}
			object, err = ws.CreateObject(ctx, "second", nil)
			world.ReleaseObjectState(object)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if counted.writes != 1 {
		t.Fatalf("write transactions = %d, want one", counted.writes)
	}
	if tx, err := engine.NewTransaction(escaped, true); err == nil {
		tx.Discard()
		t.Fatal("accepted an expired batch context")
	}

	// One failed client aborts all earlier clients in the same batch.
	err = engine.Run(t.Context(), func(ctx context.Context) error {
		if err := create(ctx, "rolled-back"); err != nil {
			return err
		}
		_ = world.ExecTransaction(ctx, engine, true, func(context.Context, world.WorldState) error {
			return errors.New("write failed")
		})
		return nil
	})
	if err == nil {
		t.Fatal("committed after a discarded write")
	}
	err = world.ExecTransaction(t.Context(), tb.Engine, false, func(ctx context.Context, ws world.WorldState) error {
		object, found, err := ws.GetObject(ctx, "rolled-back")
		world.ReleaseObjectState(object)
		if found {
			t.Error("failed batch became visible")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// batchCountEngine counts physical write admission through the real testbed.
type batchCountEngine struct {
	world.Engine
	writes int
}

func (e *batchCountEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write {
		e.writes++
	}
	return e.Engine.NewTransaction(ctx, write)
}
