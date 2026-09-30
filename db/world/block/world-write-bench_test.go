package world_block_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

func BenchmarkWorldStateSetGraphQuadWrite(b *testing.B) {
	// Open a write transaction with the two objects every quad links.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()
	ws := newWorldWriteBenchTx(ctx, b, eng)
	defer ws.Discard()
	createWorldWriteBenchObject(ctx, b, ws, "bench/set-graph/source")
	createWorldWriteBenchObject(ctx, b, ws, "bench/set-graph/target")

	// Prepare one quad with a distinct predicate per iteration.
	quads := make([]world.GraphQuad, b.N)
	for i := range quads {
		quads[i] = world.NewGraphQuadWithKeys(
			"bench/set-graph/source",
			"<bench/set-graph/predicate/"+strconv.Itoa(i)+">",
			"bench/set-graph/target",
			"",
		)
	}

	// Set one quad per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "graph_quads/op")
	counts.report(b)
}

func BenchmarkWorldStateCreateObjectWrite(b *testing.B) {
	// Open a write transaction and prepare one object key per iteration.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()
	ws := newWorldWriteBenchTx(ctx, b, eng)
	defer ws.Discard()
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = "bench/create-object/" + strconv.Itoa(i)
	}

	// Create one object per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		createWorldWriteBenchObject(opCtx, b, ws, keys[i])
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "objects/op")
	counts.report(b)
}

func BenchmarkWorldStateMultiOpWriteTransaction(b *testing.B) {
	// Build an empty World.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()

	// Prepare two object keys and the quad linking them per iteration.
	subjectKeys := make([]string, b.N)
	objectKeys := make([]string, b.N)
	quads := make([]world.GraphQuad, b.N)
	for i := range subjectKeys {
		subjectKeys[i] = "bench/multi-op/subject/" + strconv.Itoa(i)
		objectKeys[i] = "bench/multi-op/object/" + strconv.Itoa(i)
		quads[i] = world.NewGraphQuadWithKeys(
			subjectKeys[i],
			"<bench/multi-op/relates-to>",
			objectKeys[i],
			"",
		)
	}

	// Create both objects, link them, and commit one transaction per iteration.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		ws := newWorldWriteBenchTx(opCtx, b, eng)
		createWorldWriteBenchObject(opCtx, b, ws, subjectKeys[i])
		createWorldWriteBenchObject(opCtx, b, ws, objectKeys[i])
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		if err := ws.Commit(opCtx); err != nil {
			b.Fatal(err.Error())
		}
		ws.Discard()
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(2, "objects/op")
	b.ReportMetric(1, "graph_quads/op")
	b.ReportMetric(1, "world_commits/op")
	counts.report(b)
}

// Seeded World shape: every object has a type edge to one of a few hub nodes
// and a parent edge to an earlier object, so the World holds
// worldSeedObjects objects, twice as many edges, and hubs with thousands of
// incoming edges.
const (
	worldSeedObjects      = 20000
	worldSeedTypes        = 4
	worldSeedObjectsPerTx = 500
	worldSeedTypePred     = "<bench/seed/type>"
	worldSeedParentPred   = "<bench/seed/parent>"
)

// worldSeed is the seeded World shared by every seeded benchmark run in the
// process. Each run opens a fresh Engine on its root.
var worldSeed struct {
	once    sync.Once
	testbed *testbed.Testbed
	rootRef *bucket.ObjectRef
	err     error
}

func BenchmarkWorldSeededWrite(b *testing.B) {
	// Each case commits one transaction per iteration on the seeded World.
	b.Run("create-typed-object", func(b *testing.B) {
		// Create a new object in each commit.
		benchmarkWorldSeededWrite(b, func(ctx context.Context, ws world.WorldState, i int) error {
			// Create the object.
			key := "bench/seeded/new/" + strconv.Itoa(i)
			createWorldWriteBenchObject(ctx, b, ws, key)

			// Link the object to its type hub and parent.
			typeQuad := world.NewGraphQuadWithKeys(key, worldSeedTypePred, worldSeedTypeKey(i), "")
			if err := ws.SetGraphQuad(ctx, typeQuad); err != nil {
				return err
			}
			parentQuad := world.NewGraphQuadWithKeys(key, worldSeedParentPred, worldSeedObjectKey(worldSeedPick(i)), "")
			return ws.SetGraphQuad(ctx, parentQuad)
		})
	})
	b.Run("increment-object-rev", func(b *testing.B) {
		// Bump the revision of an existing object.
		benchmarkWorldSeededWrite(b, func(ctx context.Context, ws world.WorldState, i int) error {
			// Look up the object and bump its revision.
			obj, err := world.MustGetObject(ctx, ws, worldSeedObjectKey(worldSeedPick(i)))
			if err != nil {
				return err
			}
			defer world.ReleaseObjectState(obj)
			_, err = obj.IncrementRev(ctx)
			return err
		})
	})
	b.Run("link-existing-objects", func(b *testing.B) {
		// Link two existing objects with a new edge.
		benchmarkWorldSeededWrite(b, func(ctx context.Context, ws world.WorldState, i int) error {
			quad := world.NewGraphQuadWithKeys(
				worldSeedObjectKey(worldSeedPick(i)),
				"<bench/seeded/link>",
				worldSeedObjectKey(worldSeedPick(i+worldSeedObjects/2)),
				"",
			)
			return ws.SetGraphQuad(ctx, quad)
		})
	})
}

// benchmarkWorldSeededWrite applies op in one committed transaction per
// iteration on a fresh Engine over the seeded World.
func benchmarkWorldSeededWrite(b *testing.B, op func(ctx context.Context, ws world.WorldState, i int) error) {
	// Open an Engine on the seeded World.
	ctx := context.Background()
	eng, cleanup := setupWorldSeededBench(ctx, b)
	defer cleanup()

	// Apply and commit op once per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		ws := newWorldWriteBenchTx(opCtx, b, eng)
		if err := op(opCtx, ws, i); err != nil {
			b.Fatal(err.Error())
		}
		if err := ws.Commit(opCtx); err != nil {
			b.Fatal(err.Error())
		}
		ws.Discard()
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "world_commits/op")
	counts.report(b)
}

// setupWorldSeededBench opens a World Engine on the seeded World, seeding it
// on first use.
func setupWorldSeededBench(ctx context.Context, tb testing.TB) (*world_block.Engine, func()) {
	// Seed the shared World once per process.
	tb.Helper()
	worldSeed.once.Do(func() {
		worldSeed.testbed, worldSeed.rootRef, worldSeed.err = seedWorld(ctx)
	})
	if worldSeed.err != nil {
		tb.Fatal(worldSeed.err.Error())
	}

	// Open the Engine on a cursor at the seeded root.
	le := logrus.NewEntry(logrus.New())
	ocs, err := worldSeed.testbed.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs.SetRootRef(worldSeed.rootRef.GetRootRef())
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		ocs.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		eng.Close()
		ocs.Release()
	}
	return eng, cleanup
}

// seedWorld builds a testbed World of worldSeedObjects objects in batches of
// worldSeedObjectsPerTx and returns the testbed and the World root.
func seedWorld(ctx context.Context) (*testbed.Testbed, *bucket.ObjectRef, error) {
	// Start a testbed with an empty cursor.
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, nil, err
	}
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tbed.Release()
		return nil, nil, err
	}
	defer ocs.Release()

	// Build the Engine on the cursor.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		tbed.Release()
		return nil, nil, err
	}
	defer eng.Close()

	// Create the type hubs, then the objects in batches.
	if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
		for i := range worldSeedTypes {
			obj, err := ws.CreateObject(ctx, worldSeedTypeKey(i), nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		tbed.Release()
		return nil, nil, err
	}
	for start := 0; start < worldSeedObjects; start += worldSeedObjectsPerTx {
		if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
			return seedWorldObjects(ctx, ws, start, min(start+worldSeedObjectsPerTx, worldSeedObjects))
		}); err != nil {
			tbed.Release()
			return nil, nil, err
		}
	}
	return tbed, eng.GetRootRef(), nil
}

// seedWorldTx applies fn in one committed Engine transaction.
func seedWorldTx(ctx context.Context, eng *world_block.Engine, fn func(ws world.WorldState) error) error {
	// Open the write transaction.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Apply fn and commit.
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// seedWorldObjects creates the seed objects in [start, end) with their type
// and parent edges.
func seedWorldObjects(ctx context.Context, ws world.WorldState, start, end int) error {
	for i := start; i < end; i++ {
		// Create the object and its type edge.
		key := worldSeedObjectKey(i)
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, worldSeedTypePred, worldSeedTypeKey(i), "")); err != nil {
			return err
		}

		// Link every object after the first to an earlier parent.
		if i == 0 {
			continue
		}
		parentQuad := world.NewGraphQuadWithKeys(key, worldSeedParentPred, worldSeedObjectKey(i/2), "")
		if err := ws.SetGraphQuad(ctx, parentQuad); err != nil {
			return err
		}
	}
	return nil
}

// worldSeedObjectKey returns the key of seed object i.
func worldSeedObjectKey(i int) string {
	return "bench/seeded/object/" + strconv.Itoa(i)
}

// worldSeedTypeKey returns the type hub of seed object i.
func worldSeedTypeKey(i int) string {
	return "bench/seeded/type/" + strconv.Itoa(i%worldSeedTypes)
}

// worldSeedPick spreads iteration i across the seed objects.
func worldSeedPick(i int) int {
	return i * 7919 % worldSeedObjects
}

// setupWorldWriteBench builds a World Engine on an empty testbed bucket, as
// production does.
func setupWorldWriteBench(ctx context.Context, tb testing.TB) (*world_block.Engine, func()) {
	// Start a testbed with an empty bucket cursor.
	tb.Helper()
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Build the Engine on the cursor.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		eng.Close()
		ocs.Release()
		tbed.Release()
	}
	return eng, cleanup
}

// setupWorldState builds a writable WorldState directly on an empty testbed
// bucket, without an Engine.
func setupWorldState(ctx context.Context, tb testing.TB) (*world_block.WorldState, func()) {
	// Start a testbed with an empty bucket cursor.
	tb.Helper()
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Build a writable World on the cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tbed.Release()
	}
	return ws, cleanup
}

// newWorldWriteBenchTx opens a write transaction on the Engine.
func newWorldWriteBenchTx(ctx context.Context, tb testing.TB, eng *world_block.Engine) world.Tx {
	// Open the transaction or fail the benchmark.
	tb.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err.Error())
	}
	return tx
}

// createWorldWriteBenchObject creates an empty object and releases its state.
func createWorldWriteBenchObject(ctx context.Context, tb testing.TB, ws world.WorldState, key string) {
	// Create the object and release its state.
	tb.Helper()
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		tb.Fatal(err.Error())
	}
}

// worldWriteBenchCounts accumulates block reads and writes across iterations.
type worldWriteBenchCounts struct {
	readCount, readBytes   uint64
	writeCount, writeBytes uint64
}

// withWorldWriteBenchCounters returns a context that counts block reads and writes.
func withWorldWriteBenchCounters(ctx context.Context) (context.Context, *block.ReadCounter, *block.WriteCounter) {
	ctx, readCounter := block.WithReadCounter(ctx)
	ctx, writeCounter := block.WithWriteCounter(ctx)
	return ctx, readCounter, writeCounter
}

// add adds one iteration's counter values.
func (c *worldWriteBenchCounts) add(readCounter *block.ReadCounter, writeCounter *block.WriteCounter) {
	// Add the read and write snapshots to the totals.
	reads := readCounter.Snapshot()
	writes := writeCounter.Snapshot()
	c.readCount += reads.BlockReadCount
	c.readBytes += reads.BlockReadBytes
	c.writeCount += writes.BlockWriteCount
	c.writeBytes += writes.BlockWriteBytes
}

// report reports per-iteration block read and write metrics.
func (c *worldWriteBenchCounts) report(b *testing.B) {
	// Report nothing when no iterations ran.
	if b.N == 0 {
		return
	}

	// Report block reads and writes per iteration.
	denom := float64(b.N)
	b.ReportMetric(float64(c.readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(c.readBytes)/denom, "block-read-bytes/op")
	b.ReportMetric(float64(c.writeCount)/denom, "block-writes/op")
	b.ReportMetric(float64(c.writeBytes)/denom, "block-write-bytes/op")
}
