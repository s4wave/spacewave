package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

func BenchmarkWorldStateLookupGraphQuadsBatchRelationshipFanout(b *testing.B) {
	ctx := context.Background()
	ws, filters, cleanup := setupRelationshipFanoutBenchWorld(ctx, b, 96)
	defer cleanup()

	b.Run("primitive-loop", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			var total int
			for _, filter := range filters {
				quads, err := ws.LookupGraphQuads(ctx, filter, 16)
				if err != nil {
					b.Fatal(err.Error())
				}
				total += len(quads)
			}
			if total != len(filters) {
				b.Fatalf("result count = %d, want %d", total, len(filters))
			}
		}
	})

	b.Run("owner-batch", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			results, err := ws.LookupGraphQuadsBatch(ctx, filters, 16)
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
		}
	})
}

func setupRelationshipFanoutBenchWorld(ctx context.Context, b *testing.B, roots int) (*world_block.WorldState, []world.GraphQuad, func()) {
	b.Helper()

	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		b.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		b.Fatal(err.Error())
	}
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tb.Release()
		b.Fatal(err.Error())
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
			tb.Release()
			b.Fatal(err.Error())
		}
		for predIndex, pred := range outPredicates {
			targetKey := rootKey + "/out/" + strconv.Itoa(predIndex)
			if _, err := ws.CreateObject(ctx, targetKey, nil); err != nil {
				ws.Discard()
				ocs.Release()
				tb.Release()
				b.Fatal(err.Error())
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(rootKey, pred, targetKey, "")); err != nil {
				ws.Discard()
				ocs.Release()
				tb.Release()
				b.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys(rootKey, pred, "", ""))
		}
		for predIndex, pred := range inPredicates {
			sourceKey := rootKey + "/in/" + strconv.Itoa(predIndex)
			if _, err := ws.CreateObject(ctx, sourceKey, nil); err != nil {
				ws.Discard()
				ocs.Release()
				tb.Release()
				b.Fatal(err.Error())
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(sourceKey, pred, rootKey, "")); err != nil {
				ws.Discard()
				ocs.Release()
				tb.Release()
				b.Fatal(err.Error())
			}
			filters = append(filters, world.NewGraphQuadWithKeys("", pred, rootKey, ""))
		}
	}

	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tb.Release()
	}
	return ws, filters, cleanup
}
