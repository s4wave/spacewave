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

type graphEdgeBucketFilterTarget struct {
	bucket   *GraphEdgeBucket
	incoming bool
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
	filters := make([]GraphQuad, 0, len(query.OriginObjectKeys)*2)
	targets := make([]graphEdgeBucketFilterTarget, 0, len(query.OriginObjectKeys)*2)
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
			filters = append(filters, NewGraphQuadWithKeys(origin, query.Predicate, "", ""))
			targets = append(targets, graphEdgeBucketFilterTarget{bucket: bucket})
		}
		if query.Direction == GraphEdgeBucketDirectionIn || query.Direction == GraphEdgeBucketDirectionBoth {
			filters = append(filters, NewGraphQuadWithKeys("", query.Predicate, origin, ""))
			targets = append(targets, graphEdgeBucketFilterTarget{bucket: bucket, incoming: true})
		}
		buckets[i] = bucket
	}

	if len(filters) != 0 {
		results, err := ws.LookupGraphQuadsBatch(ctx, filters, 0)
		if err != nil {
			return nil, err
		}
		for i, quads := range results {
			sortGraphQuads(quads)
			truncated := uint64(len(quads)) > uint64(query.LimitPerOrigin)
			if truncated {
				quads = quads[:query.LimitPerOrigin]
			}
			target := targets[i]
			if target.incoming {
				target.bucket.Incoming = quads
				target.bucket.IncomingTruncated = truncated
				continue
			}
			target.bucket.Outgoing = quads
			target.bucket.OutgoingTruncated = truncated
		}
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
