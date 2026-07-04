package world_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/world"
)

func TestWorldStateSetGraphQuadUsesBatchCollectorBeforeObjectLookup(t *testing.T) {
	ctx := context.Background()
	gq := world.NewGraphQuadWithKeys(
		"batch-exists/source",
		"<batch-exists/predicate>",
		"batch-exists/target",
		"",
	)
	cq, err := world.GraphQuadToCayleyQuad(gq, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	store := &batchExistenceQuadStore{results: [][]quad.Quad{{cq}}}
	ws := &WorldState{
		write:   true,
		graphHd: &cayley.Handle{QuadStore: store},
	}

	if err := ws.SetGraphQuad(ctx, gq); err != nil {
		t.Fatal(err.Error())
	}
	if store.collectCalls != 1 {
		t.Fatalf("CollectFilteredQuadsBatch calls = %d, want 1", store.collectCalls)
	}
	if store.limit != 1 {
		t.Fatalf("CollectFilteredQuadsBatch limit = %d, want 1", store.limit)
	}
	if len(store.filters) != 1 || !world.QuadEqual(store.filters[0], cq) {
		t.Fatalf("CollectFilteredQuadsBatch filters = %#v, want %#v", store.filters, []quad.Quad{cq})
	}
}

type batchExistenceQuadStore struct {
	graph.QuadStore

	collectCalls int
	filters      []quad.Quad
	limit        uint32
	results      [][]quad.Quad
}

func (s *batchExistenceQuadStore) CollectFilteredQuadsBatch(ctx context.Context, filters []quad.Quad, limitPerFilter uint32) ([][]quad.Quad, error) {
	s.collectCalls++
	s.filters = append(s.filters[:0], filters...)
	s.limit = limitPerFilter
	return s.results, nil
}
