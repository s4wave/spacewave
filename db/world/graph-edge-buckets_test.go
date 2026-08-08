package world_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

func TestListGraphEdgeBucketsOrdersLimitsAndTruncates(t *testing.T) {
	ctx := context.Background()
	graph := &edgeBucketTestGraph{
		quads: []world.GraphQuad{
			world.NewGraphQuadWithKeys("origin-a", "<rel-z>", "target-z", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-y>", "target-y", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-x>", "target-x", ""),
			world.NewGraphQuadWithKeys("source-b", "<rel-b>", "origin-a", ""),
			world.NewGraphQuadWithKeys("source-a", "<rel-a>", "origin-a", ""),
			world.NewGraphQuadWithKeys("origin-b", "<rel-b>", "target-b", ""),
		},
	}

	buckets, err := world.ListGraphEdgeBuckets(ctx, graph, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{"origin-a", "origin-b"},
		LimitPerOrigin:   2,
		Direction:        world.GraphEdgeBucketDirectionBoth,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(buckets) != 2 {
		t.Fatalf("expected two buckets, got %d", len(buckets))
	}

	first := buckets[0]
	if first.OriginObjectKey != "origin-a" {
		t.Fatalf("expected first bucket for origin-a, got %q", first.OriginObjectKey)
	}
	if len(first.Outgoing) != 2 || len(first.Incoming) != 2 {
		t.Fatalf("expected limited first bucket, got outgoing=%d incoming=%d", len(first.Outgoing), len(first.Incoming))
	}
	if !first.OutgoingTruncated || first.IncomingTruncated {
		t.Fatalf("unexpected truncation state outgoing=%v incoming=%v", first.OutgoingTruncated, first.IncomingTruncated)
	}
	if got := first.Outgoing[0].GetPredicate(); got != "<rel-x>" {
		t.Fatalf("expected sorted outgoing first predicate <rel-x>, got %q", got)
	}
	if got := first.Incoming[0].GetSubject(); got != "<source-a>" {
		t.Fatalf("expected sorted incoming first subject <source-a>, got %q", got)
	}

	second := buckets[1]
	if second.OriginObjectKey != "origin-b" {
		t.Fatalf("expected second bucket for origin-b, got %q", second.OriginObjectKey)
	}
	if len(second.Outgoing) != 1 || len(second.Incoming) != 0 {
		t.Fatalf("expected one outgoing edge for second bucket, got outgoing=%d incoming=%d", len(second.Outgoing), len(second.Incoming))
	}
	if second.OutgoingTruncated || second.IncomingTruncated {
		t.Fatalf("expected second bucket not truncated, got outgoing=%v incoming=%v", second.OutgoingTruncated, second.IncomingTruncated)
	}
	if graph.lookupCalls != 0 {
		t.Fatalf("expected bucket owner to avoid primitive lookups, got %d", graph.lookupCalls)
	}
	if graph.batchCalls != 1 || graph.batchLimit != 0 || len(graph.batchFilters) != 4 {
		t.Fatalf("unexpected batch lookup usage: calls=%d limit=%d filters=%d", graph.batchCalls, graph.batchLimit, len(graph.batchFilters))
	}
}

func TestListGraphEdgeBucketsLimitsAfterOrdering(t *testing.T) {
	ctx := context.Background()
	graph := &edgeBucketTestGraph{
		quads: []world.GraphQuad{
			world.NewGraphQuadWithKeys("origin-a", "<rel-z>", "target-z", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-y>", "target-y", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-a>", "target-a", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-b>", "target-b", ""),
		},
	}

	buckets, err := world.ListGraphEdgeBuckets(ctx, graph, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{"origin-a"},
		LimitPerOrigin:   2,
		Direction:        world.GraphEdgeBucketDirectionOut,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(buckets) != 1 {
		t.Fatalf("expected one bucket, got %d", len(buckets))
	}
	bucket := buckets[0]
	if !bucket.OutgoingTruncated {
		t.Fatal("expected outgoing bucket to be truncated")
	}
	if len(bucket.Outgoing) != 2 {
		t.Fatalf("expected two outgoing quads, got %d", len(bucket.Outgoing))
	}
	if got := bucket.Outgoing[0].GetPredicate(); got != "<rel-a>" {
		t.Fatalf("first ordered predicate: got %q want <rel-a>", got)
	}
	if got := bucket.Outgoing[1].GetPredicate(); got != "<rel-b>" {
		t.Fatalf("second ordered predicate: got %q want <rel-b>", got)
	}
}

type edgeBucketTestGraph struct {
	quads        []world.GraphQuad
	lookupCalls  int
	batchCalls   int
	batchLimit   uint32
	batchFilters []world.GraphQuad
}

func (g *edgeBucketTestGraph) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return nil
}

func (g *edgeBucketTestGraph) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	g.lookupCalls++
	return g.filterGraphQuads(filter, limit), nil
}

func (g *edgeBucketTestGraph) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	g.batchCalls++
	g.batchLimit = limitPerFilter
	g.batchFilters = append([]world.GraphQuad(nil), filters...)
	results := make([][]world.GraphQuad, len(filters))
	for i, filter := range filters {
		results[i] = g.filterGraphQuads(filter, limitPerFilter)
	}
	return results, nil
}

func (g *edgeBucketTestGraph) filterGraphQuads(filter world.GraphQuad, limit uint32) []world.GraphQuad {
	var out []world.GraphQuad
	for _, q := range g.quads {
		if filter.GetSubject() != "" && q.GetSubject() != filter.GetSubject() {
			continue
		}
		if filter.GetPredicate() != "" && q.GetPredicate() != filter.GetPredicate() {
			continue
		}
		if filter.GetObj() != "" && q.GetObj() != filter.GetObj() {
			continue
		}
		out = append(out, q)
		if limit != 0 && uint32(len(out)) >= limit {
			break
		}
	}
	return out
}

func (g *edgeBucketTestGraph) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return nil, nil
}

func (g *edgeBucketTestGraph) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return nil
}

func (g *edgeBucketTestGraph) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return nil
}

func (g *edgeBucketTestGraph) DeleteGraphObject(ctx context.Context, value string) error {
	return nil
}

var _ world.WorldStateGraph = (*edgeBucketTestGraph)(nil)
