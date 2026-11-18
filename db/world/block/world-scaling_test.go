package world_block_test

import (
	"context"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// scaleObjects is the smaller seeded World of the scaling test; the larger
// holds ten times as many objects.
const scaleObjects = 10000

// scaleCalls is the number of calls each entry point makes on each World.
const scaleCalls = 32

// scaleCost is the average cost of one call on one World.
type scaleCost struct {
	reads, writes, writeBytes, allocs float64
}

// scaleBatch is the number of adjacent objects one batch read names.
const scaleBatch = 32

// scaledWorld is one seeded World of the scaling test.
type scaledWorld struct {
	eng     *world_block.Engine
	objects int
	// runs holds, for each call, scaleBatch seed object keys adjacent in the
	// object tree, as the records of one owner are.
	runs [][]string
}

// pick returns a seed object for call i, spread over the World and distinct
// for each i below the World's size.
func (w *scaledWorld) pick(i int) int {
	return 1 + i*7919%(w.objects-1)
}

// TestWorldScaling runs each entry point of the World's cost table on a seeded
// World of n objects and one of 10n, and checks that a call's block reads and
// allocations stay flat: each should grow with the call's own quads and
// results and the depth of the World's trees, never with the World. A cost
// linear in the World grows tenfold and one in its square root threefold; one
// more tree level, which 10n adds to each path a commit rewrites and each key
// a lookup descends to, grows it by at most half, since the commit rewrites
// one more page of about 32 entries per path. The graph store's own key
// operations are exactly flat, which its package's TestScaling checks.
func TestWorldScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds two Worlds")
	}

	// Seed the two Worlds.
	ctx := t.Context()
	var worlds [2]*scaledWorld
	for i, objects := range [2]int{scaleObjects, 10 * scaleObjects} {
		tbed, rootRef, err := seedWorld(ctx, objects)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tbed.Release)
		eng, cleanup := openSeededEngine(ctx, t, tbed, rootRef)
		t.Cleanup(cleanup)
		worlds[i] = &scaledWorld{eng: eng, objects: objects}
		worlds[i].runs = adjacentRuns(t, ctx, worlds[i])
	}

	// Each entry point, as a call on one World. A write commits its own
	// transaction; a read runs in a read transaction.
	ops := []struct {
		name  string
		write bool
		op    func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error
	}{
		{"set a quad", true, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			q := world.NewGraphQuadWithKeys(
				worldSeedObjectKey(w.pick(i)),
				"<bench/scale/link>",
				worldSeedObjectKey(w.pick(i+w.objects/2)),
				"",
			)
			return ws.SetGraphQuad(ctx, q)
		}},
		{"create an object and link it", true, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			// A quad names only objects, so its new value is a new object.
			key := "bench/scale/tag/" + strconv.Itoa(i)
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(worldSeedObjectKey(w.pick(i)), "<bench/scale/tag>", key, ""))
		}},
		{"delete a quad", true, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			k := w.pick(i)
			q := world.NewGraphQuadWithKeys(worldSeedObjectKey(k), worldSeedParentPred, worldSeedObjectKey(k/2), "")
			return ws.DeleteGraphQuad(ctx, q)
		}},
		{"look up by subject and predicate", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			filter := world.NewGraphQuadWithKeys(worldSeedObjectKey(w.pick(i)), worldSeedTypePred, "", "")
			return expectQuads(ws.LookupGraphQuads(ctx, filter, 0))(1)
		}},
		{"look up by object and predicate", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			filter := world.NewGraphQuadWithKeys("", worldSeedTypePred, worldSeedTypeKey(i), "")
			return expectQuads(ws.LookupGraphQuads(ctx, filter, worldSeedResultBatch))(worldSeedResultBatch)
		}},
		{"look up one exact quad", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			k := w.pick(i)
			filter := world.NewGraphQuadWithKeys(worldSeedObjectKey(k), worldSeedTypePred, worldSeedTypeKey(k), "")
			return expectQuads(ws.LookupGraphQuads(ctx, filter, 0))(1)
		}},
		{"read the bodies of adjacent objects", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			bodies, _, err := world.GetObjectBodiesBatchPage(ctx, ws, w.runs[i], world.ObjectBodiesBatchByteBudget)
			if err == nil && len(bodies) != scaleBatch {
				err = errors.Errorf("batch read %d bodies, want %d", len(bodies), scaleBatch)
			}
			return err
		}},
		{"read the types of adjacent objects", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			metadata, err := world_types.GetObjectMetadataBatch(ctx, ws, w.runs[i])
			if err == nil && len(metadata) != scaleBatch {
				err = errors.Errorf("batch read %d types, want %d", len(metadata), scaleBatch)
			}
			return err
		}},
		{"page a type's objects oldest first", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			return expectEdgePage(ctx, ws, worldSeedTypeKey(i), "", false)
		}},
		{"page a type's objects newest first", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			return expectEdgePage(ctx, ws, worldSeedTypeKey(i), "", true)
		}},
		{"page a type's objects after one", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			k := w.pick(i) % (w.objects / 2)
			return expectEdgePage(ctx, ws, worldSeedTypeKey(k), worldSeedObjectKey(k), false)
		}},
		{"page a type's objects before one", false, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			k := w.objects/2 + w.pick(i)%(w.objects/2)
			return expectEdgePage(ctx, ws, worldSeedTypeKey(k), worldSeedObjectKey(k), true)
		}},
		{"create an object", true, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			obj, err := ws.CreateObject(ctx, "bench/scale/new/"+strconv.Itoa(i), nil)
			world.ReleaseObjectState(obj)
			return err
		}},
		{"revise an object", true, func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error {
			return bumpObjectRev(ctx, ws, worldSeedObjectKey(w.pick(i)))
		}},
	}
	for _, o := range ops {
		var costs [2]scaleCost
		for wi, w := range worlds {
			costs[wi] = measureWorld(t, ctx, w, o.write, o.op)
		}
		worldFlat(t, o.name, costs[0], costs[1])
	}

	// Diff the roots on each side of a commit that revises one object and sets
	// one quad, as a watch wake does.
	var costs [2]scaleCost
	for wi, w := range worlds {
		costs[wi] = measureWorldDiff(t, ctx, w)
	}
	worldFlat(t, "diff two roots", costs[0], costs[1])
}

// adjacentRuns returns, for each call and the warming call, scaleBatch seed
// object keys that follow one another in the object tree.
func adjacentRuns(t *testing.T, ctx context.Context, w *scaledWorld) [][]string {
	t.Helper()
	tx, err := w.eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	runs := make([][]string, scaleCalls+1)
	for i := range runs {
		iter := tx.IterateObjects(ctx, "bench/seeded/object/", false)
		if err := iter.Seek(worldSeedObjectKey(w.pick(i) % (w.objects / 2))); err != nil {
			t.Fatal(err)
		}
		for ; iter.Valid() && len(runs[i]) < scaleBatch; iter.Next() {
			runs[i] = append(runs[i], iter.Key())
		}
		err := iter.Err()
		iter.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(runs[i]) != scaleBatch || !slices.IsSorted(runs[i]) {
			t.Fatalf("run %d holds %d keys", i, len(runs[i]))
		}
	}
	return runs
}

// expectQuads returns a check that the lookup returned want quads.
func expectQuads(quads []world.GraphQuad, err error) func(want int) error {
	return func(want int) error {
		if err == nil && len(quads) != want {
			return errors.Errorf("lookup returned %d quads, want %d", len(quads), want)
		}
		return err
	}
}

// expectEdgePage lists one page of the type edges into hub through
// ListGraphEdgeBuckets and checks that it is full and truncated, and that it
// holds the hub's objects in creation order, or its reverse, starting just
// past after. The seed creates hub objects worldSeedTypes indexes apart.
func expectEdgePage(ctx context.Context, ws world.WorldState, hub, after string, reverse bool) error {
	buckets, err := ws.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{hub},
		Predicate:        worldSeedTypePred,
		LimitPerOrigin:   worldSeedResultBatch,
		Direction:        world.GraphEdgeBucketDirectionIn,
		After:            after,
		Reverse:          reverse,
	})
	if err != nil {
		return err
	}
	b := buckets[0]
	if len(b.Incoming) != worldSeedResultBatch || !b.IncomingTruncated {
		return errors.Errorf("page listed %d edges truncated %v, want %d truncated", len(b.Incoming), b.IncomingTruncated, worldSeedResultBatch)
	}

	// Check that each endpoint follows the previous one, or after, by one
	// hub step in the page's direction.
	step := worldSeedTypes
	if reverse {
		step = -step
	}
	prev := -1
	if after != "" {
		if prev, err = seedObjectIndex(after); err != nil {
			return err
		}
	}
	for _, q := range b.Incoming {
		n, err := seedObjectIndex(strings.Trim(q.GetSubject(), "<>"))
		if err != nil {
			return err
		}
		if prev >= 0 && n != prev+step {
			return errors.Errorf("page listed object %d after %d, want %d", n, prev, prev+step)
		}
		prev = n
	}
	return nil
}

// seedObjectIndex returns the index of a seed object key.
func seedObjectIndex(key string) (int, error) {
	index, ok := strings.CutPrefix(key, worldSeedObjectPrefix)
	if !ok {
		return 0, errors.Errorf("%s is not a seed object", key)
	}
	return strconv.Atoi(index)
}

// measureWorld runs op scaleCalls times on w and returns the average cost of
// a call, the transaction and a write's commit included. A first call, past
// the measured ones, warms the Engine and is not counted.
func measureWorld(
	t *testing.T,
	ctx context.Context,
	w *scaledWorld,
	write bool,
	op func(ctx context.Context, w *scaledWorld, ws world.WorldState, i int) error,
) scaleCost {
	t.Helper()
	call := func(ctx context.Context, i int) {
		tx, err := w.eng.NewTransaction(ctx, write)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		if err := op(ctx, w, tx, i); err != nil {
			t.Fatal(err)
		}
		if write {
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	call(ctx, scaleCalls)
	return measureCalls(ctx, call)
}

// measureWorldDiff commits scaleCalls changes to w and returns the average
// cost of listing the changes between the roots on each side of one.
func measureWorldDiff(t *testing.T, ctx context.Context, w *scaledWorld) scaleCost {
	t.Helper()

	// Commit each change and keep the roots around it.
	type span struct{ before, after *bucket.ObjectRef }
	spans := make([]span, scaleCalls+1)
	for i := range spans {
		before := w.eng.GetRootRef()
		tx, err := w.eng.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		k := w.pick(i)
		err = bumpObjectRev(ctx, tx, worldSeedObjectKey(k))
		if err == nil {
			err = tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(worldSeedObjectKey(k), "<bench/scale/diff>", worldSeedTypeKey(k), ""))
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		tx.Discard()
		if err != nil {
			t.Fatal(err)
		}
		spans[i] = span{before, w.eng.GetRootRef()}
	}

	// List the changes of each span, expecting the one object and quad.
	call := func(ctx context.Context, i int) {
		s := spans[i]
		err := w.eng.AccessWorldState(ctx, s.before, func(beforeCs *bucket_lookup.Cursor) error {
			return w.eng.AccessWorldState(ctx, s.after, func(afterCs *bucket_lookup.Cursor) error {
				_, beforeBcs := beforeCs.BuildTransaction(nil)
				_, afterBcs := afterCs.BuildTransaction(nil)
				changes, err := world_block.WorldChanges(ctx, beforeBcs, afterBcs, 1024)
				if err == nil && (changes.Unknown || len(changes.Keys) != 1 || len(changes.Quads) != 1) {
					err = errors.Errorf("diff listed %d keys and %d quads, want 1 and 1", len(changes.Keys), len(changes.Quads))
				}
				return err
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	call(ctx, scaleCalls)
	return measureCalls(ctx, call)
}

// measureCalls makes scaleCalls calls and returns their average block reads
// and allocations.
func measureCalls(ctx context.Context, call func(ctx context.Context, i int)) scaleCost {
	// Count the calls' block reads and allocations.
	ctx, reads := block.WithReadCounter(ctx)
	ctx, writes := block.WithWriteCounter(ctx)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range scaleCalls {
		call(ctx, i)
	}
	runtime.ReadMemStats(&after)

	// Average them over the calls.
	return scaleCost{
		reads:      float64(reads.Snapshot().BlockReadCount) / scaleCalls,
		writes:     float64(writes.Snapshot().BlockWriteCount) / scaleCalls,
		writeBytes: float64(writes.Snapshot().BlockWriteBytes) / scaleCalls,
		allocs:     float64(after.Mallocs-before.Mallocs) / scaleCalls,
	}
}

// worldFlat fails the test unless the cost at ten times the objects stays
// within twice the cost at one: room for one more tree level, and none for a
// cost that grows with the World.
func worldFlat(t *testing.T, name string, small, large scaleCost) {
	t.Helper()
	t.Logf("%s: n %+v; 10n %+v", name, small, large)
	switch {
	case large.reads > small.reads*2+0.5:
		t.Errorf("%s reads %.2f blocks a call at 10n against %.2f at n", name, large.reads, small.reads)
	case large.allocs > small.allocs*2+1:
		t.Errorf("%s allocates %.1f objects a call at 10n against %.1f at n", name, large.allocs, small.allocs)
	}
}
