package world

import (
	"context"
	"math"
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
	LimitPerOrigin uint32
	// Direction selects which edge directions are included.
	Direction GraphEdgeBucketDirection
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
}

// ListGraphEdgeBuckets lists grouped inbound/outbound graph edges for object keys.
func ListGraphEdgeBuckets(ctx context.Context, ws WorldStateGraph, query *GraphEdgeBucketQuery) ([]*GraphEdgeBucket, error) {
	if query == nil {
		return nil, nil
	}
	if query.LimitPerOrigin == 0 || query.LimitPerOrigin == math.MaxUint32 {
		return nil, ErrGraphEdgeBucketLimit
	}
	if err := validateGraphEdgeBucketDirection(query.Direction); err != nil {
		return nil, err
	}

	buckets := make([]*GraphEdgeBucket, len(query.OriginObjectKeys))
	for i, origin := range query.OriginObjectKeys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		bucket := &GraphEdgeBucket{OriginObjectKey: origin}
		if origin == "" {
			buckets[i] = bucket
			continue
		}
		if query.Direction == GraphEdgeBucketDirectionOut || query.Direction == GraphEdgeBucketDirectionBoth {
			outgoing, truncated, err := lookupGraphEdgeBucketDirection(
				ctx,
				ws,
				NewGraphQuadWithKeys(origin, query.Predicate, "", ""),
				query.LimitPerOrigin,
			)
			if err != nil {
				return nil, err
			}
			bucket.Outgoing = outgoing
			bucket.OutgoingTruncated = truncated
		}
		if query.Direction == GraphEdgeBucketDirectionIn || query.Direction == GraphEdgeBucketDirectionBoth {
			incoming, truncated, err := lookupGraphEdgeBucketDirection(
				ctx,
				ws,
				NewGraphQuadWithKeys("", query.Predicate, origin, ""),
				query.LimitPerOrigin,
			)
			if err != nil {
				return nil, err
			}
			bucket.Incoming = incoming
			bucket.IncomingTruncated = truncated
		}
		buckets[i] = bucket
	}
	return buckets, nil
}

func validateGraphEdgeBucketDirection(direction GraphEdgeBucketDirection) error {
	switch direction {
	case GraphEdgeBucketDirectionBoth, GraphEdgeBucketDirectionOut, GraphEdgeBucketDirectionIn:
		return nil
	default:
		return ErrGraphEdgeBucketDirection
	}
}

func lookupGraphEdgeBucketDirection(
	ctx context.Context,
	ws WorldStateGraph,
	filter GraphQuad,
	limit uint32,
) ([]GraphQuad, bool, error) {
	quads, err := ws.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		return nil, false, err
	}
	sortGraphQuads(quads)
	truncated := uint32(len(quads)) > limit
	if truncated {
		quads = quads[:limit]
	}
	return quads, truncated, nil
}
