package world_block_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

func TestWorldStateLookupGraphQuadsBatchMatchesPrimitiveLoop(t *testing.T) {
	// Create the relationship fanout fixture.
	ctx := context.Background()
	ws, filters, cleanup := setupRelationshipFanoutBenchWorld(ctx, t, 8)
	defer cleanup()

	// Require batch graph lookups to match primitive lookups.
	results, err := ws.LookupGraphQuadsBatch(ctx, filters, 16)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(results) != len(filters) {
		t.Fatalf("result count = %d, want %d", len(results), len(filters))
	}
	for i, filter := range filters {
		quads, err := ws.LookupGraphQuads(ctx, filter, 16)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !reflect.DeepEqual(graphQuadStrings(results[i]), graphQuadStrings(quads)) {
			t.Fatalf("filter %d batch result = %#v, want %#v", i, graphQuadStrings(results[i]), graphQuadStrings(quads))
		}
	}
}

func BenchmarkWorldStateLookupGraphQuadsBatchRelationshipFanout(b *testing.B) {
	// Create the relationship benchmark fixture.
	ctx := context.Background()
	ws, filters, cleanup := setupRelationshipFanoutBenchWorld(ctx, b, 96)
	defer cleanup()

	// Measure graph lookups one filter at a time.
	b.Run("primitive-loop", func(b *testing.B) {
		// Count block reads across primitive lookup iterations.
		b.ResetTimer()
		b.ReportAllocs()
		var readCount, readBytes uint64
		for range b.N {
			opCtx, counter := block.WithReadCounter(ctx)
			var total int
			for _, filter := range filters {
				quads, err := ws.LookupGraphQuads(opCtx, filter, 16)
				if err != nil {
					b.Fatal(err.Error())
				}
				total += len(quads)
			}
			if total != len(filters) {
				b.Fatalf("result count = %d, want %d", total, len(filters))
			}
			snapshot := counter.Snapshot()
			readCount += snapshot.BlockReadCount
			readBytes += snapshot.BlockReadBytes
		}
		reportBlockReadMetrics(b, readCount, readBytes)
	})

	// Measure graph lookups through the batch API.
	b.Run("owner-batch", func(b *testing.B) {
		// Count block reads across batch lookup iterations.
		b.ResetTimer()
		b.ReportAllocs()
		var readCount, readBytes uint64
		for range b.N {
			opCtx, counter := block.WithReadCounter(ctx)
			results, err := ws.LookupGraphQuadsBatch(opCtx, filters, 16)
			if err != nil {
				b.Fatal(err.Error())
			}
			var total int
			for _, quads := range results {
				total += len(quads)
			}
			if total != len(filters) {
				b.Fatalf("result count = %d, want %d", total, len(filters))
			}
			snapshot := counter.Snapshot()
			readCount += snapshot.BlockReadCount
			readBytes += snapshot.BlockReadBytes
		}
		reportBlockReadMetrics(b, readCount, readBytes)
	})
}

func BenchmarkWorldStateListGraphEdgeBucketsRelationshipFanout(b *testing.B) {
	// Create the graph edge bucket fixture.
	ctx := context.Background()
	const roots = 96
	ws, _, cleanup := setupRelationshipFanoutBenchWorld(ctx, b, roots)
	defer cleanup()

	// Build the query for incoming and outgoing edges.
	originKeys := make([]string, roots)
	for i := range roots {
		originKeys[i] = relationshipFanoutRootKey(i)
	}
	query := &world.GraphEdgeBucketQuery{
		OriginObjectKeys: originKeys,
		LimitPerOrigin:   16,
		Direction:        world.GraphEdgeBucketDirectionBoth,
	}

	// Measure the graph edge bucket query and its block reads.
	b.ResetTimer()
	b.ReportAllocs()
	var readCount, readBytes uint64
	for range b.N {
		opCtx, counter := block.WithReadCounter(ctx)
		buckets, err := world.ListGraphEdgeBuckets(opCtx, ws, query)
		if err != nil {
			b.Fatal(err.Error())
		}
		var total int
		for _, bucket := range buckets {
			total += len(bucket.Outgoing) + len(bucket.Incoming)
		}
		if total != roots*7 {
			b.Fatalf("result count = %d, want %d", total, roots*7)
		}
		snapshot := counter.Snapshot()
		readCount += snapshot.BlockReadCount
		readBytes += snapshot.BlockReadBytes
	}
	reportBlockReadMetrics(b, readCount, readBytes)
}

func BenchmarkWorldStateQueryGraphPathRelationshipFanout(b *testing.B) {
	ctx := context.Background()

	b.Run("existing-handle", func(b *testing.B) {
		// Measure graph paths through the existing World handle.
		ws, roots, cleanup := setupGraphPathBenchWorld(ctx, b, 96)
		defer cleanup()
		query := buildGraphPathBenchQuery(roots)
		b.ResetTimer()
		b.ReportAllocs()
		var readCount, readBytes uint64
		for range b.N {
			opCtx, counter := block.WithReadCounter(ctx)
			result, err := world.QueryGraphPathWithLookups(opCtx, ws, query)
			if err != nil {
				b.Fatal(err.Error())
			}
			if len(result.ObjectKeys) != len(roots) || len(result.Quads) != len(roots) {
				b.Fatalf("result keys=%d quads=%d, want %d", len(result.ObjectKeys), len(result.Quads), len(roots))
			}
			snapshot := counter.Snapshot()
			readCount += snapshot.BlockReadCount
			readBytes += snapshot.BlockReadBytes
		}
		reportBlockReadMetrics(b, readCount, readBytes)
	})
	b.Run("scoped-read-operation", func(b *testing.B) {
		// Measure graph paths through a scoped read operation.
		ws, roots, cleanup := setupGraphPathBenchWorld(ctx, b, 96)
		defer cleanup()
		query := buildGraphPathBenchQuery(roots)
		b.ResetTimer()
		b.ReportAllocs()
		var readCount, readBytes uint64
		for range b.N {
			opCtx, counter := block.WithReadCounter(ctx)
			result, err := ws.QueryGraphPath(opCtx, query)
			if err != nil {
				b.Fatal(err.Error())
			}
			if len(result.ObjectKeys) != len(roots) || len(result.Quads) != len(roots) {
				b.Fatalf("result keys=%d quads=%d, want %d", len(result.ObjectKeys), len(result.Quads), len(roots))
			}
			snapshot := counter.Snapshot()
			readCount += snapshot.BlockReadCount
			readBytes += snapshot.BlockReadBytes
		}
		reportBlockReadMetrics(b, readCount, readBytes)
	})
}

func buildGraphPathBenchQuery(roots []string) *world.GraphPathQuery {
	return &world.GraphPathQuery{
		StartKeys: roots,
		Steps: []world.GraphPathStep{
			{
				Direction: world.GraphPathDirectionOut,
				Predicate: "<bench/path-out>",
				Limit:     16,
			},
		},
		ResultLimit:  uint32(len(roots)), //nolint:gosec
		IncludeQuads: true,
	}
}

func setupRelationshipFanoutBenchWorld(ctx context.Context, tb testing.TB, roots int) (*world_block.WorldState, []world.GraphQuad, func()) {
	// Identify relationship fixture failures at the caller.
	tb.Helper()

	// Open a writable World for the relationship fixture.
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
	writeWs, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Create the fanout objects and their incoming and outgoing edges.
	outPredicates := []string{
		"<bench/worklist-goal>",
		"<bench/worklist-session>",
		"<bench/worklist-job>",
		"<bench/worklist-evidence>",
	}
	inPredicates := []string{
		"<bench/agent-worklist>",
		"<bench/question-worklist>",
		"<bench/wave-worklist>",
	}
	filters := make([]world.GraphQuad, 0, roots*(len(outPredicates)+len(inPredicates)))
	for i := range roots {
		rootKey := relationshipFanoutRootKey(i)
		{
			createdObject, err := writeWs.CreateObject(ctx, rootKey, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				writeWs.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
		}
		for predIndex, pred := range outPredicates {
			targetKey := rootKey + "/out/" + strconv.Itoa(predIndex)
			{
				createdObject2, err := writeWs.CreateObject(ctx, targetKey, nil)
				world.ReleaseObjectState(createdObject2)
				if err != nil {
					writeWs.Discard()
					ocs.Release()
					tbed.Release()
					tb.Fatal(err.Error())
				}
			}
			if err := writeWs.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(rootKey, pred, targetKey, "")); err != nil {
				writeWs.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys(rootKey, pred, "", ""))
		}
		for predIndex, pred := range inPredicates {
			sourceKey := rootKey + "/in/" + strconv.Itoa(predIndex)
			{
				createdObject3, err := writeWs.CreateObject(ctx, sourceKey, nil)
				world.ReleaseObjectState(createdObject3)
				if err != nil {
					writeWs.Discard()
					ocs.Release()
					tbed.Release()
					tb.Fatal(err.Error())
				}
			}
			if err := writeWs.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(sourceKey, pred, rootKey, "")); err != nil {
				writeWs.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys("", pred, rootKey, ""))
		}
	}
	if err := writeWs.Commit(ctx); err != nil {
		writeWs.Discard()
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	ocs.SetRootRef(writeWs.GetRootRef())
	writeWs.Discard()

	// Open a read-only World on the committed fanout.
	readWs, err := world_block.BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Return the fanout World with its cleanup.
	cleanup := func() {
		readWs.Discard()
		ocs.Release()
		tbed.Release()
	}
	return readWs, filters, cleanup
}

func relationshipFanoutRootKey(i int) string {
	return "bench/worklist/" + strconv.Itoa(i)
}

func setupGraphPathBenchWorld(ctx context.Context, tb testing.TB, roots int) (*world_block.WorldState, []string, func()) {
	// Identify graph path fixture failures at the caller.
	tb.Helper()

	// Open a writable World for the graph path fixture.
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
	writeWs, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Create root objects with one outgoing edge each.
	rootKeys := make([]string, roots)
	for i := range roots {
		rootKey := "bench/path/root/" + strconv.Itoa(i)
		targetKey := rootKey + "/target"
		rootKeys[i] = rootKey
		{
			createdObject, err := writeWs.CreateObject(ctx, rootKey, nil)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				writeWs.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
		}
		{
			createdObject2, err := writeWs.CreateObject(ctx, targetKey, nil)
			world.ReleaseObjectState(createdObject2)
			if err != nil {
				writeWs.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
		}
		if err := writeWs.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(rootKey, "<bench/path-out>", targetKey, "")); err != nil {
			writeWs.Discard()
			ocs.Release()
			tbed.Release()
			tb.Fatal(err.Error())
		}
	}
	if err := writeWs.Commit(ctx); err != nil {
		writeWs.Discard()
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	ocs.SetRootRef(writeWs.GetRootRef())
	writeWs.Discard()

	// Return the read-only graph path World with its cleanup.
	readWs, err := world_block.BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		readWs.Discard()
		ocs.Release()
		tbed.Release()
	}
	return readWs, rootKeys, cleanup
}

func graphQuadStrings(quads []world.GraphQuad) []string {
	out := make([]string, len(quads))
	for i, q := range quads {
		out[i] = q.GetSubject() + "\x00" + q.GetPredicate() + "\x00" + q.GetObj() + "\x00" + q.GetLabel()
	}
	return out
}

func reportBlockReadMetrics(b *testing.B, readCount, readBytes uint64) {
	if b.N == 0 {
		return
	}
	denom := float64(b.N)
	b.ReportMetric(float64(readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(readBytes)/denom, "block-read-bytes/op")
}
