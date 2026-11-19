package world

import "context"

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

// graphPathStepFilters returns the quad filters one step reads from key: its
// outgoing, then its incoming relationships under the step's predicate.
func graphPathStepFilters(key string, step GraphPathStep) []GraphQuad {
	var filters []GraphQuad
	if step.Direction == GraphPathDirectionOut || step.Direction == GraphPathDirectionBoth {
		filters = append(filters, NewGraphQuadWithKeys(key, step.Predicate, "", ""))
	}
	if step.Direction == GraphPathDirectionIn || step.Direction == GraphPathDirectionBoth {
		filters = append(filters, NewGraphQuadWithKeys("", step.Predicate, key, ""))
	}
	return filters
}

// lookupGraphPathStep reads each of a step's filters with its own limit and
// merges the directions in field order.
func lookupGraphPathStep(ctx context.Context, ws WorldStateGraph, key string, step GraphPathStep) ([]GraphQuad, error) {
	var quads []GraphQuad
	for _, filter := range graphPathStepFilters(key, step) {
		found, err := ws.LookupGraphQuads(ctx, filter, step.Limit)
		if err != nil {
			return nil, err
		}
		quads = append(quads, found...)
	}
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
