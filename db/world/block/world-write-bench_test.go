package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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

// setupWorldState builds a WorldState that tracks the World-local GC graph
// directly on an empty testbed bucket.
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
