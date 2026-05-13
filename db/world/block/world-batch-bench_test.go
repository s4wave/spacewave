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
	ctx := context.Background()
	ws, filters, cleanup := setupRelationshipFanoutBenchWorld(ctx, t, 8)
	defer cleanup()

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
	ctx := context.Background()
	ws, filters, cleanup := setupRelationshipFanoutBenchWorld(ctx, b, 96)
	defer cleanup()

	b.Run("primitive-loop", func(b *testing.B) {
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

	b.Run("owner-batch", func(b *testing.B) {
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

func setupRelationshipFanoutBenchWorld(ctx context.Context, tb testing.TB, roots int) (*world_block.WorldState, []world.GraphQuad, func()) {
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

	outPredicates := []string{
		"<bench/workfront-attractor>",
		"<bench/workfront-session>",
		"<bench/workfront-job>",
		"<bench/workfront-evidence>",
	}
	inPredicates := []string{
		"<bench/agent-workfront>",
		"<bench/question-workfront>",
		"<bench/wave-workfront>",
	}
	filters := make([]world.GraphQuad, 0, roots*(len(outPredicates)+len(inPredicates)))
	for i := range roots {
		rootKey := "bench/workfront/" + strconv.Itoa(i)
		if _, err := ws.CreateObject(ctx, rootKey, nil); err != nil {
			ws.Discard()
			ocs.Release()
			tbed.Release()
			tb.Fatal(err.Error())
		}
		for predIndex, pred := range outPredicates {
			targetKey := rootKey + "/out/" + strconv.Itoa(predIndex)
			if _, err := ws.CreateObject(ctx, targetKey, nil); err != nil {
				ws.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(rootKey, pred, targetKey, "")); err != nil {
				ws.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys(rootKey, pred, "", ""))
		}
		for predIndex, pred := range inPredicates {
			sourceKey := rootKey + "/in/" + strconv.Itoa(predIndex)
			if _, err := ws.CreateObject(ctx, sourceKey, nil); err != nil {
				ws.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(sourceKey, pred, rootKey, "")); err != nil {
				ws.Discard()
				ocs.Release()
				tbed.Release()
				tb.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys("", pred, rootKey, ""))
		}
	}

	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tbed.Release()
	}
	return ws, filters, cleanup
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
