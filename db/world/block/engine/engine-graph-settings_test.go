package world_block_engine_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/quad"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// TestWorldEngineControllerGraphSettings checks that a local World created
// from a config with graph settings starts with them in its root.
func TestWorldEngineControllerGraphSettings(t *testing.T) {
	// Start the engine with a predicate index.
	settings := predicateSettings(t)
	ctx, _, _, startEngine := newEngineTestbed(t)
	eng, engRef := startEngine(true, func(c *world_block_engine.Config) {
		c.GraphSettings = settings
	})
	defer engRef.Release()

	// The World root records the settings.
	if got := rootGraphSettings(t, ctx, eng); !got.EqualVT(settings) {
		t.Fatalf("world root graph settings %v", got)
	}
}

// TestWorldEngineControllerChangesGraphSettings checks that restarting an
// existing World's engine with new graph settings moves the World to them in
// the background, and that listing the default indexes moves it back, with
// lookups unchanged throughout.
func TestWorldEngineControllerChangesGraphSettings(t *testing.T) {
	// Start a default World.
	ctx, _, _, startEngine := newEngineTestbed(t)
	eng, engRef := startEngine(true)
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Link objects under three predicates.
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
	if len(want) != 66 {
		t.Fatalf("wrote %d links", len(want))
	}
	engRef.Release()

	// Restart with the predicate index and wait for the World to reach it.
	settings := predicateSettings(t)
	eng, engRef = startEngine(true, func(c *world_block_engine.Config) {
		c.GraphSettings = settings
	})
	waitRootGraphSettings(t, ctx, eng, settings)
	if got := lookupPredicates(t, ctx, eng); !slices.Equal(got, want) {
		t.Fatalf("lookups with the index: %v", got)
	}
	engRef.Release()

	// Restart with the default indexes listed and wait for the World to drop
	// the index.
	defaults, err := cayley_kv.QuadIndexMetas(cayley_kv.DefaultQuadIndexes)
	if err != nil {
		t.Fatal(err.Error())
	}
	eng, engRef = startEngine(true, func(c *world_block_engine.Config) {
		c.GraphSettings = &world_block.GraphSettings{Indexes: defaults}
	})
	defer engRef.Release()
	waitRootGraphSettings(t, ctx, eng, nil)
	if got := lookupPredicates(t, ctx, eng); !slices.Equal(got, want) {
		t.Fatalf("lookups after the drop: %v", got)
	}
}

// predicateSettings returns graph settings holding the default indexes plus
// one serving lookups that pin only the predicate.
func predicateSettings(t *testing.T) *world_block.GraphSettings {
	// Extend the default indexes and wrap them in settings.
	t.Helper()
	indexes := append(slices.Clone(cayley_kv.DefaultQuadIndexes), cayley_kv.QuadIndex{
		Dirs: []quad.Direction{quad.Predicate, quad.Label, quad.Subject},
	})
	settings, err := world_block.NewGraphSettings(indexes)
	if err != nil {
		t.Fatal(err.Error())
	}
	return settings
}

// rootGraphSettings decodes the engine's World root and returns its settings.
func rootGraphSettings(t *testing.T, ctx context.Context, eng world.Engine) *world_block.GraphSettings {
	// Fail the test on any read error.
	t.Helper()
	settings, err := readRootGraphSettings(ctx, eng)
	if err != nil {
		t.Fatal(err.Error())
	}
	return settings
}

// readRootGraphSettings decodes the engine's World root and returns its
// settings.
func readRootGraphSettings(ctx context.Context, eng world.Engine) (*world_block.GraphSettings, error) {
	// Decode the root through the engine's World state access.
	var settings *world_block.GraphSettings
	err := eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		// Unmarshal the root block and keep its settings.
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(ctx, world_block.NewWorldBlock)
		if err != nil {
			return err
		}
		settings = wi.(*world_block.World).GetGraphSettings()
		return nil
	})
	return settings, err
}

// waitRootGraphSettings waits for the engine's World root to hold settings.
func waitRootGraphSettings(t *testing.T, ctx context.Context, eng world.Engine, settings *world_block.GraphSettings) {
	// Poll the root until it matches or the deadline passes. Right after a
	// restart the lookup can still resolve the previous engine as it shuts
	// down: keep polling through its close until the new engine serves.
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := readRootGraphSettings(ctx, eng)
		if err != nil && !errors.Is(err, world_block.ErrEngineClosed) {
			t.Fatal(err.Error())
		}
		if err == nil && got.EqualVT(settings) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("world root graph settings %v, want %v", got, settings)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// lookupPredicates returns the sorted links under the three test predicates.
func lookupPredicates(t *testing.T, ctx context.Context, eng world.Engine) []string {
	// Look up each predicate in one read transaction.
	t.Helper()
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	var out []string
	for p := range 3 {
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuad("", fmt.Sprintf("<settings/rel-%d>", p), "", ""), 0)
		if err != nil {
			t.Fatal(err.Error())
		}
		for _, q := range quads {
			out = append(out, q.GetSubject()+" "+q.GetPredicate()+" "+q.GetObj())
		}
	}
	slices.Sort(out)
	return out
}
