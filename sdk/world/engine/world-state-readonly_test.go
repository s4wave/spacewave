//go:build !js

package sdk_world_engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// TestSDKWorldStateReadOnlyWrites preserves the sentinel error before any write RPC.
func TestSDKWorldStateReadOnlyWrites(t *testing.T) {
	// Start the real SDK engine.
	ctx := t.Context()
	engine, cleanup := setupSDKEngine(ctx, t)
	t.Cleanup(cleanup)

	// Open a writer to seed the object used by the read transaction.
	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writeTx.Discard)

	// Commit an object that the read transaction can acquire.
	created, err := writeTx.CreateObject(ctx, "readonly/object", nil)
	world.ReleaseObjectState(created)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Acquire the existing object through a read-only SDK transaction.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readTx.Discard)
	obj, found, err := readTx.GetObject(ctx, "readonly/object")
	t.Cleanup(func() { world.ReleaseObjectState(obj) })
	if err != nil || !found {
		t.Fatalf("read object: found=%v err=%v", found, err)
	}

	// Cancel only the mutation context so an attempted RPC cannot mask a local refusal.
	writeCtx, cancel := context.WithCancel(ctx)
	cancel()
	quad := world.NewGraphQuadWithKeys("readonly/object", "<relation>", "readonly/object", "")
	writes := []struct {
		// method identifies the mutation under test.
		method string
		// write attempts a mutation with a canceled RPC context.
		write func() error
	}{
		{"CreateObject", func() error {
			obj, err := readTx.CreateObject(writeCtx, "readonly/new", nil)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"RenameObject", func() error {
			obj, err := readTx.RenameObject(writeCtx, "readonly/object", "readonly/renamed", false)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"DeleteObject", func() error {
			_, err := readTx.DeleteObject(writeCtx, "readonly/object")
			return err
		}},
		{"StageWorldState", func() error {
			stage, err := readTx.StageWorldState(writeCtx)
			if stage != nil {
				stage.Release()
			}
			return err
		}},
		{"SetGraphQuad", func() error { return readTx.SetGraphQuad(writeCtx, quad) }},
		{"DeleteGraphQuad", func() error { return readTx.DeleteGraphQuad(writeCtx, quad) }},
		{"DeleteGraphObject", func() error { return readTx.DeleteGraphObject(writeCtx, "readonly/object") }},
		{"AccessCayleyGraph", func() error { return readTx.AccessCayleyGraph(writeCtx, true, nil) }},
		{"ApplyWorldOp", func() error {
			_, _, err := readTx.ApplyWorldOp(writeCtx, nil, "")
			return err
		}},
		{"SetRootRef", func() error {
			_, err := obj.SetRootRef(writeCtx, nil)
			return err
		}},
		{"IncrementRev", func() error {
			_, err := obj.IncrementRev(writeCtx)
			return err
		}},
		{"ApplyObjectOp", func() error {
			_, _, err := obj.ApplyObjectOp(writeCtx, nil, "")
			return err
		}},
	}

	// Require every mutation to preserve the transaction sentinel for replaying callers.
	for _, write := range writes {
		if err := write.write(); !errors.Is(err, tx.ErrNotWrite) {
			t.Fatalf("%s error = %v, want tx.ErrNotWrite", write.method, err)
		}
	}
}
