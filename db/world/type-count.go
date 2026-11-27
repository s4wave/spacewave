package world

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
)

// typePredicateValue is the graph value of the type predicate.
var typePredicateValue = quad.IRI(TypePredicate).String()

// TypeEdgeTypeID returns the type ID declared by a quad with predicate pred and
// object obj, as graph values, when the quad is a type edge: the <type>
// predicate to a types/<id> key. ok is false for any other quad.
func TypeEdgeTypeID(pred, obj string) (typeID string, ok bool) {
	// Match the type predicate, then a key under the type prefix.
	if pred != typePredicateValue {
		return "", false
	}
	key, err := GraphValueToKey(obj)
	if err != nil {
		return "", false
	}
	typeID, ok = strings.CutPrefix(key, TypeObjectKeyPrefix)
	return typeID, ok && typeID != ""
}

// AddTypeCounts returns new counts holding counts plus deltas, sorted by type
// ID, without the types whose count reaches zero. A count never goes below
// zero. counts must be sorted by type ID. Neither argument is changed.
func AddTypeCounts(counts []*TypeCount, deltas map[string]int64) []*TypeCount {
	// Sum the deltas into a copy of the counts.
	sums := make(map[string]int64, len(counts)+len(deltas))
	for _, c := range counts {
		sums[c.GetTypeId()] += int64(c.GetCount()) //nolint:gosec // counts stay far below 2^63.
	}
	for typeID, d := range deltas {
		sums[typeID] += d
	}

	// Keep the positive sums in type ID order.
	out := make([]*TypeCount, 0, len(sums))
	for typeID, n := range sums {
		if n > 0 {
			out = append(out, &TypeCount{TypeId: typeID, Count: uint64(n)})
		}
	}
	slices.SortFunc(out, func(a, b *TypeCount) int { return cmp.Compare(a.GetTypeId(), b.GetTypeId()) })
	return out
}

// CountTypeQuads returns the type counts of a set of distinct quads.
func CountTypeQuads(quads []GraphQuad) []*TypeCount {
	deltas := make(map[string]int64)
	for _, q := range quads {
		if typeID, ok := TypeEdgeTypeID(q.GetPredicate(), q.GetObj()); ok {
			deltas[typeID]++
		}
	}
	return AddTypeCounts(nil, deltas)
}

// DiffTypeCounts returns the counts of next that differ from prev, and a zero
// count for each type of prev absent from next, sorted by type ID. Both
// arguments must be sorted by type ID.
func DiffTypeCounts(prev, next []*TypeCount) []*TypeCount {
	var out []*TypeCount
	i, j := 0, 0
	for i < len(prev) || j < len(next) {
		var c int
		switch {
		case i == len(prev):
			c = 1
		case j == len(next):
			c = -1
		default:
			c = cmp.Compare(prev[i].GetTypeId(), next[j].GetTypeId())
		}
		switch {
		case c < 0:
			out = append(out, &TypeCount{TypeId: prev[i].GetTypeId()})
			i++
		case c > 0:
			out = append(out, next[j])
			j++
		default:
			if prev[i].GetCount() != next[j].GetCount() {
				out = append(out, next[j])
			}
			i++
			j++
		}
	}
	return out
}

// WatchTypeCounts calls fn with every type count of ws and the seqno they
// were read at, then waits for each change of the seqno and calls fn with the
// counts that moved, as DiffTypeCounts returns them. It reads the counts once
// per change and calls fn only when one moved. It returns when ctx ends or a
// read or fn fails.
func WatchTypeCounts(ctx context.Context, ws WorldState, fn func(seqno uint64, counts []*TypeCount) error) error {
	var prev []*TypeCount
	for first := true; ; first = false {
		// Read the seqno before the counts, so a change between the two
		// reads wakes the wait below at once instead of being missed.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return err
		}
		counts, err := ws.ListTypeCounts(ctx)
		if err != nil {
			return err
		}

		// Send all counts first, then only the moved ones.
		send := counts
		if !first {
			send = DiffTypeCounts(prev, counts)
		}
		if first || len(send) != 0 {
			if err := fn(seqno, send); err != nil {
				return err
			}
		}
		prev = counts

		// Wait for the next change.
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// LookupTypeCount returns the count of typeID in counts sorted by type ID.
func LookupTypeCount(counts []*TypeCount, typeID string) uint64 {
	i, found := slices.BinarySearchFunc(counts, typeID, func(c *TypeCount, id string) int {
		return cmp.Compare(c.GetTypeId(), id)
	})
	if !found {
		return 0
	}
	return counts[i].GetCount()
}
