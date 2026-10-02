package world

import (
	"cmp"
	"context"
	"slices"
	"strings"
)

// GraphPathDirection indicates which side of the current object key to follow.
type GraphPathDirection int

const (
	// GraphPathDirectionOut follows quads where the current key is the subject.
	GraphPathDirectionOut GraphPathDirection = iota + 1
	// GraphPathDirectionIn follows quads where the current key is the object.
	GraphPathDirectionIn
	// GraphPathDirectionBoth follows incoming and outgoing quads.
	GraphPathDirectionBoth
)

// GraphPathStep is one bounded predicate traversal step.
type GraphPathStep struct {
	// Direction is the edge direction to follow.
	Direction GraphPathDirection
	// Predicate is the graph predicate to match.
	Predicate string
	// Limit is the maximum number of quads inspected per current key.
	Limit uint32
}

// GraphPathQuery describes a bounded server-executed graph traversal.
type GraphPathQuery struct {
	// StartKeys is the list of object keys where traversal starts.
	StartKeys []string
	// Steps is the ordered traversal to execute.
	Steps []GraphPathStep
	// ResultLimit is the maximum number of final object keys returned.
	ResultLimit uint32
	// IncludeQuads includes traversed quads in the result.
	IncludeQuads bool
}

// GraphPathQueryResult contains graph path traversal results.
type GraphPathQueryResult struct {
	// ObjectKeys is the set of reached object keys.
	ObjectKeys []string
	// Quads is the set of traversed quads when requested.
	Quads []GraphQuad
}

// CollectGraphPathWithKeys collects object keys for a bounded graph traversal.
func CollectGraphPathWithKeys(ctx context.Context, ws WorldStateGraph, query *GraphPathQuery) ([]string, error) {
	// Preserve the traversal result while returning only its reached object keys.
	result, err := ws.QueryGraphPath(ctx, query)
	if err != nil || result == nil {
		return nil, err
	}
	return result.ObjectKeys, nil
}

// CollectGraphPathStepWithKeys collects object keys after one bounded predicate step.
func CollectGraphPathStepWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	direction GraphPathDirection,
	predicate string,
	limit uint32,
) ([]string, error) {
	// An empty frontier requires no graph access or bounded query construction.
	if len(entityKeys) == 0 {
		return nil, nil
	}

	// Use the same bounded query contract for this single traversal step.
	return CollectGraphPathWithKeys(ctx, ws, &GraphPathQuery{
		StartKeys: entityKeys,
		Steps: []GraphPathStep{
			{
				Direction: direction,
				Predicate: predicate,
				Limit:     limit,
			},
		},
		ResultLimit: limit,
	})
}

// QueryGraphPathWithLookups executes a graph path query with bounded quad lookups.
func QueryGraphPathWithLookups(ctx context.Context, ws WorldStateGraph, query *GraphPathQuery) (*GraphPathQueryResult, error) {
	// Validate all bounds before any graph operation can observe the query.
	if query == nil {
		return &GraphPathQueryResult{}, nil
	}
	if query.ResultLimit == 0 {
		return nil, ErrGraphPathResultLimit
	}
	if err := validateGraphPathSteps(query.Steps); err != nil {
		return nil, err
	}

	// Select the initial frontier in request order, then traverse it in key order.
	current := uniqueNonEmptyKeys(query.StartKeys, query.ResultLimit)
	if len(query.Steps) == 0 {
		return &GraphPathQueryResult{ObjectKeys: current}, nil
	}

	// Complete field tuples retain distinct relationships containing separator bytes.
	quadSeen := make(map[[4]string]struct{})
	var resultQuads []GraphQuad
	for _, step := range query.Steps {
		// A step receives the prior frontier and independently deduplicates reached keys.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nextSeen := make(map[string]struct{})
		var next []string
		for _, key := range current {
			// Apply each direction's limit before considering the next frontier.
			quads, err := lookupGraphPathStep(ctx, ws, key, step)
			if err != nil {
				return nil, err
			}
			for _, q := range quads {
				// Include each complete relationship once across the whole traversal.
				if query.IncludeQuads {
					qkey := [4]string{q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()}
					if _, ok := quadSeen[qkey]; !ok {
						quadSeen[qkey] = struct{}{}
						resultQuads = append(resultQuads, q)
					}
				}

				// A new endpoint consumes the bounded frontier capacity exactly once.
				nextKey, err := nextGraphPathKey(q, key, step.Direction)
				if err != nil {
					return nil, err
				}
				if nextKey == "" {
					continue
				}
				if _, ok := nextSeen[nextKey]; ok {
					continue
				}
				nextSeen[nextKey] = struct{}{}
				next = append(next, nextKey)
				if uint32(len(next)) >= query.ResultLimit { //nolint:gosec // traversal results are bounded by the uint32 ResultLimit field.
					break
				}
			}
			if uint32(len(next)) >= query.ResultLimit { //nolint:gosec // traversal results are bounded by the uint32 ResultLimit field.
				break
			}
		}

		// Canonical key order determines the next step's bounded visitation order.
		slices.Sort(next)
		current = next
	}

	// Return the included relationships in field order.
	sortGraphQuads(resultQuads)
	return &GraphPathQueryResult{ObjectKeys: current, Quads: resultQuads}, nil
}

// validateGraphPathSteps requires finite access and a defined direction at each step.
func validateGraphPathSteps(steps []GraphPathStep) error {
	for _, step := range steps {
		if step.Predicate == "" {
			return ErrGraphPathPredicate
		}
		if step.Limit == 0 {
			return ErrGraphPathStepLimit
		}
		switch step.Direction {
		case GraphPathDirectionOut, GraphPathDirectionIn, GraphPathDirectionBoth:
		default:
			return ErrGraphPathDirection
		}
	}
	return nil
}

// uniqueNonEmptyKeys selects distinct keys before sorting the bounded selection.
func uniqueNonEmptyKeys(keys []string, limit uint32) []string {
	// The input order determines which keys occupy the available result slots.
	seen := make(map[string]struct{})
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
		if limit != 0 && uint32(len(out)) >= limit { //nolint:gosec // final results are bounded by the uint32 query limit.
			break
		}
	}

	// Subsequent graph operations visit the selected keys in canonical order.
	slices.Sort(out)
	return out
}

// lookupGraphPathStep combines independently bounded direction lookups in public order.
func lookupGraphPathStep(ctx context.Context, ws WorldStateGraph, key string, step GraphPathStep) ([]GraphQuad, error) {
	// Read outgoing relationships through the World's existing lookup capability.
	var quads []GraphQuad
	if step.Direction == GraphPathDirectionOut || step.Direction == GraphPathDirectionBoth {
		out, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys(key, step.Predicate, "", ""), step.Limit)
		if err != nil {
			return nil, err
		}
		quads = append(quads, out...)
	}

	// Incoming lookup has its own limit, including when both directions were requested.
	if step.Direction == GraphPathDirectionIn || step.Direction == GraphPathDirectionBoth {
		in, err := ws.LookupGraphQuads(ctx, NewGraphQuadWithKeys("", step.Predicate, key, ""), step.Limit)
		if err != nil {
			return nil, err
		}
		quads = append(quads, in...)
	}

	// Merge both directions into field order.
	sortGraphQuads(quads)
	return quads, nil
}

// nextGraphPathKey chooses the opposite object endpoint for the requested direction.
func nextGraphPathKey(q GraphQuad, currentKey string, dir GraphPathDirection) (string, error) {
	switch dir {
	case GraphPathDirectionOut:
		return GraphValueToKey(q.GetObj())
	case GraphPathDirectionIn:
		return GraphValueToKey(q.GetSubject())
	case GraphPathDirectionBoth:
		subject, err := GraphValueToKey(q.GetSubject())
		if err != nil || subject != currentKey {
			return subject, err
		}
		return GraphValueToKey(q.GetObj())
	default:
		return "", ErrGraphPathDirection
	}
}

// sortGraphQuads orders quads by subject, predicate, object, then label.
func sortGraphQuads(quads []GraphQuad) {
	slices.SortFunc(quads, func(a, b GraphQuad) int {
		return cmp.Or(
			strings.Compare(a.GetSubject(), b.GetSubject()),
			strings.Compare(a.GetPredicate(), b.GetPredicate()),
			strings.Compare(a.GetObj(), b.GetObj()),
			strings.Compare(a.GetLabel(), b.GetLabel()),
		)
	})
}
