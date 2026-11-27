package world

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
)

// EdgePager is a graph store that pages the edges joining one origin by one
// predicate from a cursor, reading only the page.
type EdgePager interface {
	graph.QuadStore
	// IterateEdges calls cb with the endpoint and label of each quad that
	// joins origin by pred: its object when origin is its subject (dir is
	// quad.Subject), its subject when origin is its object (quad.Object). The
	// label is nil when the quad has none. Edges come in the order the
	// endpoint nodes were created, reversed when reverse, with each
	// endpoint's edges together. The
	// walk starts just past all edges of the endpoint after, or at the first
	// edge when after is nil, and stops when cb returns false or an error.
	// IterateEdges returns the error.
	IterateEdges(ctx context.Context, origin, pred graph.Ref, dir quad.Direction, after graph.Ref, reverse bool, cb func(endpoint, label graph.Ref) (bool, error)) error
}

// PageGraphEdgeBuckets lists the query's edge buckets through pager, reading
// one page per origin direction, in endpoint creation order. The query must
// select a predicate.
func PageGraphEdgeBuckets(ctx context.Context, pager EdgePager, query *GraphEdgeBucketQuery) ([]*GraphEdgeBucket, error) {
	// Resolve the cursor and predicate: an after with no node is gone, and a
	// predicate with no node has no edges.
	if err := query.Validate(); err != nil {
		return nil, err
	}
	if query.Predicate == "" {
		return nil, errors.New("paging graph edge buckets requires a predicate")
	}
	var afterRef graph.Ref
	if query.After != "" {
		var err error
		afterRef, err = pager.ValueOf(ctx, GraphQuadStringToCayleyValue(quad.IRI(query.After).String()))
		if err != nil {
			return nil, err
		}
		if afterRef == nil {
			return nil, ErrGraphEdgeBucketAfterGone
		}
	}
	buckets := newGraphEdgeBuckets(query)
	predRef, err := pager.ValueOf(ctx, GraphQuadStringToCayleyValue(query.Predicate))
	if err != nil || predRef == nil {
		return buckets, err
	}

	// Page each origin's requested directions.
	p := edgePage{pager: pager, pred: query.Predicate, predRef: predRef, after: afterRef, reverse: query.Reverse, limit: query.LimitPerOrigin}
	for _, bucket := range buckets {
		bucket.CreationOrder = true
		if bucket.OriginObjectKey == "" {
			continue
		}
		origin := quad.IRI(bucket.OriginObjectKey).String()
		originRef, err := pager.ValueOf(ctx, GraphQuadStringToCayleyValue(origin))
		if err != nil {
			return nil, err
		}
		if originRef == nil {
			continue
		}
		if query.Direction != GraphEdgeBucketDirectionIn {
			bucket.Outgoing, bucket.OutgoingTruncated, err = p.read(ctx, origin, originRef, quad.Subject)
			if err != nil {
				return nil, err
			}
		}
		if query.Direction != GraphEdgeBucketDirectionOut {
			bucket.Incoming, bucket.IncomingTruncated, err = p.read(ctx, origin, originRef, quad.Object)
			if err != nil {
				return nil, err
			}
		}
	}
	return buckets, nil
}

// edgePage reads one page of a predicate's edges for each origin.
type edgePage struct {
	pager   EdgePager
	pred    string
	predRef graph.Ref
	after   graph.Ref
	reverse bool
	limit   uint32
}

// read returns the page of origin's edges where origin is in direction dir,
// and whether edges remain past it. The page ends at an endpoint boundary,
// so the labeled edges of its last endpoint may carry it past the limit.
func (p *edgePage) read(ctx context.Context, origin string, originRef graph.Ref, dir quad.Direction) ([]GraphQuad, bool, error) {
	// Walk the edges, building each page edge and finding where the page ends.
	var quads []GraphQuad
	var last graph.Ref
	var lastName string
	var more bool
	err := p.pager.IterateEdges(ctx, originRef, p.predRef, dir, p.after, p.reverse, func(endpoint, label graph.Ref) (bool, error) {
		// Stop at the first new endpoint once the page is full.
		if last == nil || endpoint.Key() != last.Key() {
			if len(quads) >= int(p.limit) {
				more = true
				return false, nil
			}
			value, err := p.pager.NameOf(ctx, endpoint)
			if err != nil {
				return false, err
			}
			last, lastName = endpoint, GraphValueToString(value)
		}

		// Decode the label and build the edge with origin in its direction.
		var labelName string
		if label != nil {
			value, err := p.pager.NameOf(ctx, label)
			if err != nil {
				return false, err
			}
			labelName = GraphValueToString(value)
		}
		if dir == quad.Subject {
			quads = append(quads, NewGraphQuad(origin, p.pred, lastName, labelName))
		} else {
			quads = append(quads, NewGraphQuad(lastName, p.pred, origin, labelName))
		}
		return true, nil
	})
	return quads, more, err
}
