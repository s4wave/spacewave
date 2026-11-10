package world_block_engine_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// TestWorldEngineControllerRegenerates checks that restarting a cayley kv
// World's engine with a regeneration target moves the World to the node graph
// in the background with its lookups unchanged, and that the regenerated World
// takes writes.
func TestWorldEngineControllerRegenerates(t *testing.T) {
	// Start a kv World and link objects under three predicates.
	ctx, _, _, startEngine := newEngineTestbed(t)
	eng, engRef := startEngine(true)
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for i := range 12 {
		key := fmt.Sprintf("settings/obj-%d", i)
		obj, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		for j := range i {
			q := world.NewGraphQuadWithKeys(key, fmt.Sprintf("<settings/rel-%d>", (i+j)%3), fmt.Sprintf("settings/obj-%d", j), "")
			if err := tx.SetGraphQuad(ctx, q); err != nil {
				t.Fatal(err.Error())
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	want := lookupPredicates(t, ctx, eng)
	if f := rootFormat(t, ctx, eng); f != world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		t.Fatalf("new World format %v", f)
	}
	engRef.Release()

	// Restart with the regeneration target and wait for the node graph head.
	eng, engRef = startEngine(true, func(c *world_block_engine.Config) {
		c.Regen = &world_block_engine.RegenConfig{
			Target:   world_block_engine.RegenTarget_REGEN_TARGET_DONE,
			SpillDir: t.TempDir(),
		}
	})
	defer engRef.Release()
	waitRootWorld(t, ctx, eng, func(w *world_block.World) bool {
		return w.GetFormat() == world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH
	})
	if got := lookupPredicates(t, ctx, eng); !slices.Equal(got, want) {
		t.Fatalf("lookups after regeneration: %v", got)
	}

	// Link one more pair on the regenerated World.
	tx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	q := world.NewGraphQuadWithKeys("settings/obj-0", "<settings/rel-0>", "settings/obj-11", "")
	if err := tx.SetGraphQuad(ctx, q); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if got := lookupPredicates(t, ctx, eng); len(got) != len(want)+1 {
		t.Fatalf("%d links after a write, want %d", len(got), len(want)+1)
	}
}

// rootFormat returns the format of the engine's World root.
func rootFormat(t *testing.T, ctx context.Context, eng world.Engine) world_block.WorldFormat {
	t.Helper()
	w, err := readRootWorld(ctx, eng)
	if err != nil {
		t.Fatal(err.Error())
	}
	return w.GetFormat()
}
