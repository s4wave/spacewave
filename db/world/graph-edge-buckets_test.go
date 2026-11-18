package world_test

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

func TestListGraphEdgeBucketsOrdersLimitsAndTruncates(t *testing.T) {
	// Populate a graph with incoming and outgoing edges for two origins.
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

	// List bounded edge buckets for both graph directions.
	buckets, err := graph.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
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

	// Verify ordering and truncation for the first origin bucket.
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

	// Verify the sparse second origin bucket and shared batch lookup.
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
	// Populate outgoing graph edges whose insertion and lexical orders differ.
	ctx := context.Background()
	graph := &edgeBucketTestGraph{
		quads: []world.GraphQuad{
			world.NewGraphQuadWithKeys("origin-a", "<rel-z>", "target-z", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-y>", "target-y", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-a>", "target-a", ""),
			world.NewGraphQuadWithKeys("origin-a", "<rel-b>", "target-b", ""),
		},
	}

	// Request a bounded outgoing edge bucket for the origin.
	buckets, err := graph.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
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

	// Verify that the outgoing limit retains the earliest ordered predicates.
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

func TestListGraphEdgeBucketsResumesAfter(t *testing.T) {
	// Link the origin to four targets by one predicate, one by two labels.
	ctx := context.Background()
	graph := &edgeBucketTestGraph{
		quads: []world.GraphQuad{
			world.NewGraphQuadWithKeys("origin", "<rel>", "target-d", ""),
			world.NewGraphQuadWithKeys("origin", "<rel>", "target-b", "<l2>"),
			world.NewGraphQuadWithKeys("origin", "<rel>", "target-a", ""),
			world.NewGraphQuadWithKeys("origin", "<rel>", "target-b", "<l1>"),
			world.NewGraphQuadWithKeys("origin", "<rel>", "target-c", ""),
		},
	}

	// page lists one page and returns its endpoints and labels.
	page := func(after string, reverse bool) ([]string, bool) {
		buckets, err := graph.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
			OriginObjectKeys: []string{"origin"},
			Predicate:        "<rel>",
			LimitPerOrigin:   1,
			Direction:        world.GraphEdgeBucketDirectionOut,
			After:            after,
			Reverse:          reverse,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		var got []string
		for _, q := range buckets[0].Outgoing {
			got = append(got, q.GetObj()+q.GetLabel())
		}
		return got, buckets[0].OutgoingTruncated
	}

	// A page past an endpoint holds every edge of its one endpoint, either way.
	if got, more := page("target-a", false); !slices.Equal(got, []string{"<target-b><l1>", "<target-b><l2>"}) || !more {
		t.Fatalf("page after target-a: got %v more=%v", got, more)
	}
	if got, more := page("target-c", true); !slices.Equal(got, []string{"<target-b><l2>", "<target-b><l1>"}) || !more {
		t.Fatalf("reverse page after target-c: got %v more=%v", got, more)
	}
	if got, more := page("target-c", false); !slices.Equal(got, []string{"<target-d>"}) || more {
		t.Fatalf("last page: got %v more=%v", got, more)
	}

	// Resuming needs a predicate to page.
	_, err := graph.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{"origin"}, LimitPerOrigin: 1, After: "target-a",
	})
	if err != world.ErrGraphEdgeBucketAfter {
		t.Fatalf("after without predicate: got %v", err)
	}

	// Resuming after an endpoint no quad names reports it gone, as the
	// node graph pager does.
	_, err = graph.ListGraphEdgeBuckets(ctx, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: []string{"origin"}, Predicate: "<rel>", LimitPerOrigin: 1, After: "target-gone",
	})
	if err != world.ErrGraphEdgeBucketAfterGone {
		t.Fatalf("after a gone endpoint: got %v", err)
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
	// Record the graph batch request and return matching quads for each filter.
	g.batchCalls++
	g.batchLimit = limitPerFilter
	g.batchFilters = append([]world.GraphQuad(nil), filters...)
	results := make([][]world.GraphQuad, len(filters))
	for i, filter := range filters {
		results[i] = g.filterGraphQuads(filter, limitPerFilter)
	}
	return results, nil
}

func (g *edgeBucketTestGraph) ListGraphEdgeBuckets(ctx context.Context, query *world.GraphEdgeBucketQuery) ([]*world.GraphEdgeBucket, error) {
	return world.CollectGraphEdgeBuckets(ctx, g, query)
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
