package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

func BenchmarkWorldStateSetGraphQuadWrite(b *testing.B) {
	ctx := context.Background()
	ws, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()

	if _, err := ws.CreateObject(ctx, "bench/set-graph/source", nil); err != nil {
		b.Fatal(err.Error())
	}
	if _, err := ws.CreateObject(ctx, "bench/set-graph/target", nil); err != nil {
		b.Fatal(err.Error())
	}
	quads := make([]world.GraphQuad, b.N)
	for i := range quads {
		quads[i] = world.NewGraphQuadWithKeys(
			"bench/set-graph/source",
			"<bench/set-graph/predicate/"+strconv.Itoa(i)+">",
			"bench/set-graph/target",
			"",
		)
	}

	b.ReportAllocs()
	b.ResetTimer()
	var readCount, readBytes uint64
	for i := range b.N {
		opCtx, counter := block.WithReadCounter(ctx)
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		snapshot := counter.Snapshot()
		readCount += snapshot.BlockReadCount
		readBytes += snapshot.BlockReadBytes
	}
	b.ReportMetric(1, "graph_quads/op")
	reportWorldWriteBlockReadMetrics(b, readCount, readBytes)
}

func BenchmarkWorldStateCreateObjectWrite(b *testing.B) {
	ctx := context.Background()
	ws, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()

	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = "bench/create-object/" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	var readCount, readBytes uint64
	for i := range b.N {
		opCtx, counter := block.WithReadCounter(ctx)
		if _, err := ws.CreateObject(opCtx, keys[i], nil); err != nil {
			b.Fatal(err.Error())
		}
		snapshot := counter.Snapshot()
		readCount += snapshot.BlockReadCount
		readBytes += snapshot.BlockReadBytes
	}
	b.ReportMetric(1, "objects/op")
	reportWorldWriteBlockReadMetrics(b, readCount, readBytes)
}

func BenchmarkWorldStateMultiOpWriteTransaction(b *testing.B) {
	ctx := context.Background()
	ws, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()

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

	b.ReportAllocs()
	b.ResetTimer()
	var readCount, readBytes uint64
	for i := range b.N {
		opCtx, counter := block.WithReadCounter(ctx)
		if _, err := ws.CreateObject(opCtx, subjectKeys[i], nil); err != nil {
			b.Fatal(err.Error())
		}
		if _, err := ws.CreateObject(opCtx, objectKeys[i], nil); err != nil {
			b.Fatal(err.Error())
		}
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		if err := ws.Commit(opCtx); err != nil {
			b.Fatal(err.Error())
		}
		snapshot := counter.Snapshot()
		readCount += snapshot.BlockReadCount
		readBytes += snapshot.BlockReadBytes
	}
	b.ReportMetric(2, "objects/op")
	b.ReportMetric(1, "graph_quads/op")
	b.ReportMetric(1, "world_commits/op")
	reportWorldWriteBlockReadMetrics(b, readCount, readBytes)
}

func setupWorldWriteBench(ctx context.Context, tb testing.TB) (*world_block.WorldState, func()) {
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

func reportWorldWriteBlockReadMetrics(b *testing.B, readCount, readBytes uint64) {
	if b.N == 0 {
		return
	}
	denom := float64(b.N)
	b.ReportMetric(float64(readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(readBytes)/denom, "block-read-bytes/op")
}
