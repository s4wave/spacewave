package world

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
)

// GraphEdgeBucketDirection indicates which edge directions to list.
type GraphEdgeBucketDirection int

const (
	// GraphEdgeBucketDirectionBoth lists incoming and outgoing edges.
	GraphEdgeBucketDirectionBoth GraphEdgeBucketDirection = iota
	// GraphEdgeBucketDirectionOut lists only outgoing edges.
	GraphEdgeBucketDirectionOut
	// GraphEdgeBucketDirectionIn lists only incoming edges.
	GraphEdgeBucketDirectionIn
)

// GraphEdgeBucketQuery describes grouped graph edge bucket lookup.
type GraphEdgeBucketQuery struct {
	// OriginObjectKeys are object keys whose graph edges should be grouped.
	OriginObjectKeys []string
	// Predicate optionally restricts grouped edges.
	Predicate string
	// LimitPerOrigin is the maximum edge count returned per origin direction.
	// A page ends at an endpoint boundary, so the labeled edges of its last
	// endpoint may carry it past the limit.
	LimitPerOrigin uint32
	// Direction selects which edge directions are included.
	Direction GraphEdgeBucketDirection
	// After resumes each listing just past all edges of this endpoint object
	// key, the last endpoint of the previous page. It requires a Predicate.
	After string
	// Reverse lists each direction's edges last first.
	Reverse bool
}

// Validate checks that the query is bounded and its fields agree.
func (q *GraphEdgeBucketQuery) Validate() error {
	if q.LimitPerOrigin == 0 || q.LimitPerOrigin == math.MaxUint32 {
		return ErrGraphEdgeBucketLimit
	}
	switch q.Direction {
	case GraphEdgeBucketDirectionBoth, GraphEdgeBucketDirectionOut, GraphEdgeBucketDirectionIn:
	default:
		return ErrGraphEdgeBucketDirection
	}
	if q.After != "" && q.Predicate == "" {
		return ErrGraphEdgeBucketAfter
	}
	return nil
}

// GraphEdgeBucket contains grouped graph edges for one origin object key.
type GraphEdgeBucket struct {
	// OriginObjectKey is the requested origin object key.
	OriginObjectKey string
	// Outgoing contains quads where the origin is the subject.
	Outgoing []GraphQuad
	// Incoming contains quads where the origin is the object.
	Incoming []GraphQuad
	// OutgoingTruncated indicates more outgoing edges matched than were returned.
	OutgoingTruncated bool
	// IncomingTruncated indicates more incoming edges matched than were returned.
	IncomingTruncated bool
	// CreationOrder reports that each direction's edges come in the order
	// their endpoint objects were created, oldest first or newest first when
	// the query is Reverse. Otherwise they come in endpoint key text order, and
	// a reader that needs creation order must read every page.
	CreationOrder bool
}

// newGraphEdgeBuckets returns one empty bucket per origin, in query order.
func newGraphEdgeBuckets(query *GraphEdgeBucketQuery) []*GraphEdgeBucket {
	buckets := make([]*GraphEdgeBucket, len(query.OriginObjectKeys))
	for i, origin := range query.OriginObjectKeys {
		buckets[i] = &GraphEdgeBucket{OriginObjectKey: origin}
	}
	return buckets
}

// graphEdgeBucketFilterTarget is the bucket direction a lookup filter fills.
type graphEdgeBucketFilterTarget struct {
	bucket   *GraphEdgeBucket
	incoming bool
}

// graphEdgeBucketFilters returns the lookup filter of each non-empty origin
// direction the query lists, and the bucket direction each fills.
func graphEdgeBucketFilters(query *GraphEdgeBucketQuery, buckets []*GraphEdgeBucket) ([]GraphQuad, []graphEdgeBucketFilterTarget) {
	filters := make([]GraphQuad, 0, len(buckets)*2)
	targets := make([]graphEdgeBucketFilterTarget, 0, len(buckets)*2)
	for _, bucket := range buckets {
		origin := bucket.OriginObjectKey
		if origin == "" {
			continue
		}
		if query.Direction != GraphEdgeBucketDirectionIn {
			filters = append(filters, NewGraphQuadWithKeys(origin, query.Predicate, "", ""))
			targets = append(targets, graphEdgeBucketFilterTarget{bucket: bucket})
		}
		if query.Direction != GraphEdgeBucketDirectionOut {
			filters = append(filters, NewGraphQuadWithKeys("", query.Predicate, origin, ""))
			targets = append(targets, graphEdgeBucketFilterTarget{bucket: bucket, incoming: true})
		}
	}
	return filters, targets
}

// CollectGraphEdgeBuckets lists the query's edge buckets by fetching every
// matching edge of each origin direction, then ordering them by quad text,
// filtering past After and truncating to the page. Its cost grows with each
// origin's edges; an EdgePager reads only the page.
func CollectGraphEdgeBuckets(ctx context.Context, ws WorldStateGraph, query *GraphEdgeBucketQuery) ([]*GraphEdgeBucket, error) {
	// Require a bounded graph edge query.
	if query == nil {
		return nil, nil
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// Require the After endpoint to still be named by some quad, as a node
	// graph requires its node.
	if query.After != "" {
		named, err := ws.LookupGraphQuadsBatch(ctx, []GraphQuad{
			NewGraphQuadWithKeys(query.After, "", "", ""),
			NewGraphQuadWithKeys("", "", query.After, ""),
		}, 1)
		if err != nil {
			return nil, err
		}
		if len(named[0]) == 0 && len(named[1]) == 0 {
			return nil, ErrGraphEdgeBucketAfterGone
		}
	}

	// Build graph quad filters and their destination buckets for each origin.
	buckets := newGraphEdgeBuckets(query)
	filters, targets := graphEdgeBucketFilters(query, buckets)
	if len(filters) == 0 {
		return buckets, nil
	}

	// Collect matching graph quads and cut each ordered edge list to its page.
	results, err := ws.LookupGraphQuadsBatch(ctx, filters, 0)
	if err != nil {
		return nil, err
	}
	var after string
	if query.After != "" {
		after = quad.IRI(query.After).String()
	}
	for i, quads := range results {
		target := targets[i]
		quads, truncated := pageGraphEdges(quads, target.incoming, after, query.Reverse, query.LimitPerOrigin)
		if target.incoming {
			target.bucket.Incoming = quads
			target.bucket.IncomingTruncated = truncated
			continue
		}
		target.bucket.Outgoing = quads
		target.bucket.OutgoingTruncated = truncated
	}
	return buckets, nil
}

// pageGraphEdges orders one origin direction's edges, drops those up to and
// including the endpoint after, and keeps limit edges plus the further edges
// of the last kept endpoint. It reports whether edges remain past the page.
func pageGraphEdges(quads []GraphQuad, incoming bool, after string, reverse bool, limit uint32) ([]GraphQuad, bool) {
	// Order the edges by endpoint, then label.
	sortGraphQuads(quads)
	if reverse {
		slices.Reverse(quads)
	}
	endpoint := func(q GraphQuad) string {
		if incoming {
			return q.GetSubject()
		}
		return q.GetObj()
	}

	// Skip to the first endpoint past after.
	if after != "" {
		start, _ := slices.BinarySearchFunc(quads, after, func(q GraphQuad, after string) int {
			c := strings.Compare(endpoint(q), after)
			if reverse {
				c = -c
			}
			if c == 0 {
				return -1
			}
			return c
		})
		quads = quads[start:]
	}

	// End the page at the endpoint boundary at or past the limit.
	end := min(uint64(len(quads)), uint64(limit))
	for end != 0 && end < uint64(len(quads)) && endpoint(quads[end]) == endpoint(quads[end-1]) {
		end++
	}
	return quads[:end], end < uint64(len(quads))
}
