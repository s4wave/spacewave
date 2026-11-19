package space_world_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/testbed"
)

// measureBound runs TestRegenerateWorldStallBound, which measures time.
var measureBound = flag.Bool("regen-bound", false, "measure the World regeneration stall bound")

// TestRegenerateWorldStallBound measures, on a testbed Space with more links
// than one graph settings step indexes, how long a write waits while replay
// applies the largest graph settings step, maxGraphSettingsStep links, and
// while it applies a regeneration of the same World. The first is the bound a
// replayed step may hold the writers for. It runs only with -regen-bound.
func TestRegenerateWorldStallBound(t *testing.T) {
	if !*measureBound {
		t.Skip("pass -regen-bound to measure the regeneration stall bound")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()
	tb, local := startLocalProvider(ctx, t)

	// Create a Space and serve its World.
	owner, _ := newReplicaSession(ctx, t, local)
	ref, err := owner.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	so, releaseObject, err := owner.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	eng, served := startReplicaEngine(ctx, t, tb, ref, "regen-bound")

	// Seed objects and links past one step's bound, in large transactions.
	const objects, perTx = 256, 4096
	links := 65536 + 8192
	start := time.Now()
	for i := range objects {
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, fmt.Sprintf("o-%d", i), nil)
			world.ReleaseObjectState(obj)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for n := 0; n < links; n += perTx {
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			for i := n; i < n+perTx; i++ {
				q := world.NewGraphQuadWithKeys(fmt.Sprintf("o-%d", i%objects), fmt.Sprintf("<p%d>", i/(objects*objects)), fmt.Sprintf("o-%d", (i/objects)%objects), "")
				if err := ws.SetGraphQuad(ctx, q); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("seeded %d objects and %d links in %v", objects, links, time.Since(start))

	// Measure a writer alone, then while the graph settings step by the
	// largest step.
	_, idle := measureWrites(ctx, t, eng, "idle", func() error {
		time.Sleep(2 * time.Second)
		return nil
	})
	t.Logf("idle: writes %v", idle)
	target, err := world_block.NewGraphSettings(append(slices.Clone(cayley_kv.DefaultQuadIndexes), cayley_kv.QuadIndex{
		Dirs: []quad.Direction{quad.Predicate, quad.Label, quad.Subject},
	}))
	if err != nil {
		t.Fatal(err)
	}
	took, stats := measureWrites(ctx, t, eng, "step", func() error {
		_, err := served.StepGraphSettings(ctx, target, 65536)
		return err
	})
	t.Logf("graph settings step of 65536 links: call %v, writes %v", took, stats)

	// Write the bound's quads, all missing from the World, as RegenerateWorld
	// carries them, then pin with none, build off the replay path, and
	// switch. The carry's longest write is its largest chunk's hold.
	bound := space_world.MaxGraphSettingsStep
	carried := make([]world.GraphQuad, 0, bound)
	for i := range bound {
		carried = append(carried, world.NewGraphQuadWithKeys(fmt.Sprintf("o-%d", i%objects), fmt.Sprintf("<carried%d>", i/(objects*objects)), fmt.Sprintf("o-%d", (i/objects)%objects), ""))
	}
	graphFormat := world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH
	list := writeHeldQuadList(ctx, t, so.GetBlockStore(), eng, carried...)
	took, stats = measureWrites(ctx, t, eng, "carry", func() error {
		return space_world.WriteCarriedQuads(ctx, served, list)
	})
	t.Logf("carry of %d quads in writes of at most %d: call %v, writes %v", bound, space_world.RegenCarryQuads, took, stats)
	took, stats = measureWrites(ctx, t, eng, "pin", func() error {
		return space_world.WriteRegenOp(ctx, served, &space_world.RegenerateWorldOp{Target: graphFormat})
	})
	t.Logf("empty pin: call %v, writes %v", took, stats)
	took, stats = measureWrites(ctx, t, eng, "build", func() error {
		invalid, err := space_world.WaitRegenBuild(ctx, served)
		if err == nil && invalid != "" {
			err = errors.New(invalid)
		}
		return err
	})
	t.Logf("off-path build of %d links: call %v, writes %v", links+bound, took, stats)
	switchTo := func() error {
		return space_world.WriteRegenOp(ctx, served, &space_world.RegenerateWorldOp{Target: graphFormat, Base: space_world.HeldRegenPin(served)})
	}
	took, stats = measureWrites(ctx, t, eng, "switch", switchTo)
	t.Logf("switch with the prebuilt root: call %v, writes %v", took, stats)

	// Switch back, stop the builder, and regenerate again: the switch builds
	// inline, as on a replica whose builder lags.
	if err := space_world.WriteRegenOp(ctx, served, &space_world.RegenerateWorldOp{Target: world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV}); err != nil {
		t.Fatal(err)
	}
	space_world.StopRegenBuild(served)
	if err := space_world.WriteRegenOp(ctx, served, &space_world.RegenerateWorldOp{Target: graphFormat}); err != nil {
		t.Fatal(err)
	}
	for space_world.HeldRegenPin(served) == nil {
		time.Sleep(10 * time.Millisecond)
	}
	took, stats = measureWrites(ctx, t, eng, "inline", switchTo)
	t.Logf("switch building inline: call %v, writes %v", took, stats)
}

// writeStats summarizes the latencies of a writer's commits.
type writeStats struct {
	// n is the number of writes.
	n int
	// p50, p99 and max are the latency quantiles.
	p50, p99, max time.Duration
}

// String formats the stats.
func (s writeStats) String() string {
	return fmt.Sprintf("n=%d p50=%v p99=%v max=%v", s.n, s.p50, s.p99, s.max)
}

// measureWrites runs op while a writer writes one object and quad at a time,
// and returns op's duration and the latencies of the writes that began while
// it ran.
func measureWrites(ctx context.Context, t *testing.T, eng world.Engine, name string, op func() error) (time.Duration, writeStats) {
	t.Helper()
	var mu sync.Mutex
	var lat []time.Duration
	measuring := false
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			mu.Lock()
			counted := measuring
			mu.Unlock()
			key := fmt.Sprintf("%s-write-%d", name, i)
			began := time.Now()
			if err := writeObjectQuad(ctx, eng, key, world.NewGraphQuadWithKeys(key, "<written>", "o-0", "")); err != nil {
				t.Error(err)
				return
			}
			if counted {
				mu.Lock()
				lat = append(lat, time.Since(began))
				mu.Unlock()
			}
		}
	})

	// Let the writer reach a steady pace, then run op.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	measuring = true
	mu.Unlock()
	began := time.Now()
	err := op()
	took := time.Since(began)
	mu.Lock()
	measuring = false
	mu.Unlock()
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(lat)
	stats := writeStats{n: len(lat)}
	if len(lat) != 0 {
		stats.p50, stats.p99, stats.max = lat[len(lat)/2], lat[len(lat)*99/100], lat[len(lat)-1]
	}
	return took, stats
}

// startLocalProvider starts a testbed with the local provider and the Space
// World engine and returns the provider.
func startLocalProvider(ctx context.Context, t *testing.T) (*testbed.Testbed, *provider_local.Provider) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))
	_, providerController, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local", PeerId: tb.Volume.GetPeerID().String(), StorageId: tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerController.Release)
	rawProvider, providerRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(providerRef.Release)
	return tb, rawProvider.(*provider_local.Provider)
}
