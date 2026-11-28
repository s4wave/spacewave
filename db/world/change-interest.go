package world

import "slices"

// ChangeInterest is a ChangeFilter that also records the changes it may accept,
// so a change feed can index its watcher by those names and test it only on a
// change that selects one of them. An interest without names, or one marked as
// needing every change, is tested on every change. A change feed reads the
// names once, when the watch starts, so a filter that may later accept other
// changes must start a new watch.
type ChangeInterest struct {
	// filter decides whether a change concerns the watcher.
	filter ChangeFilter
	// keys are object keys the filter accepts every change to. A change feed
	// hands such a change to the watcher without testing the filter.
	keys []string
	// terms are the changed quad fields the filter may accept.
	terms []quadTerm
	// all is set when the filter may accept a change referencing none of
	// the keys or terms.
	all bool
	// shared are finished read sets whose changes the filter may accept, and
	// does accept for every change touching an object key a set read. A
	// change feed indexes each set once for all the watchers sharing it, so
	// a watcher costs one entry per set, not one per key the set read.
	shared []*ReadSet
	// named selects the entries of a change a named interest holds, or is
	// nil for any other interest.
	named *namedSelect
}

// quadField selects one field of a graph quad.
type quadField uint8

const (
	// quadSubject is the subject field.
	quadSubject quadField = iota
	// quadPredicate is the predicate field.
	quadPredicate
	// quadObj is the object field.
	quadObj
	// quadLabel is the label field.
	quadLabel
)

// quadTerm is one field value of a graph quad, spelled as the graph stores it.
type quadTerm struct {
	// field is the quad field.
	field quadField
	// value is the field's value.
	value string
}

// NewChangeInterest returns an interest that filters changes with filter and
// records none of them, so a change feed tests it on every change.
func NewChangeInterest(filter ChangeFilter) *ChangeInterest {
	return &ChangeInterest{filter: filter, all: true}
}

// NewReadSetsInterest returns an interest in the changes that touch any of
// sets, finished read sets. A ChangeIndex holds each set once for all the
// watchers referencing it, so watchers built from shared sets cost one entry
// per set.
func NewReadSetsInterest(sets ...*ReadSet) *ChangeInterest {
	return &ChangeInterest{
		filter: func(changes *ChangeSet) bool {
			return slices.ContainsFunc(sets, func(set *ReadSet) bool { return set.Touched(changes) })
		},
		shared: sets,
	}
}

// Accepts reports whether the interest's filter lets changes through. A nil
// interest accepts every change.
func (c *ChangeInterest) Accepts(changes *ChangeSet) bool {
	return c == nil || c.filter.Accepts(changes)
}

// needsEveryChange reports whether the index must consider the interest on
// every change.
func (c *ChangeInterest) needsEveryChange() bool {
	return c == nil || c.all
}

// visitSharedSets visits the declared sets that the index must hold.
func (c *ChangeInterest) visitSharedSets(visit func(*ReadSet)) {
	if c.needsEveryChange() {
		return
	}
	for _, set := range c.shared {
		visit(set)
	}
}

// visitReleasedSets visits the held sets the next interest does not need.
func (c *ChangeInterest) visitReleasedSets(next *ChangeInterest, visit func(*ReadSet)) {
	// Only a named interest holds sets.
	if c.needsEveryChange() {
		return
	}

	// Index the retained sets once instead of comparing every pair.
	keep := make(map[*ReadSet]struct{})
	if !next.needsEveryChange() {
		for _, set := range next.shared {
			keep[set] = struct{}{}
		}
	}

	// Release each set the next interest omits.
	for _, set := range c.shared {
		if _, ok := keep[set]; !ok {
			visit(set)
		}
	}
}

// visitNames visits a named interest's direct object keys and quad terms.
func (c *ChangeInterest) visitNames(key func(string), term func(quadTerm)) {
	for _, name := range c.keys {
		key(name)
	}
	for _, name := range c.terms {
		term(name)
	}
}

// visitNameChanges visits the direct names added or removed when one named
// interest replaces another.
func (c *ChangeInterest) visitNameChanges(next *ChangeInterest, addKey, removeKey func(string), addTerm, removeTerm func(quadTerm)) {
	visitNameChanges(c.keys, next.keys, addKey, removeKey)
	visitNameChanges(c.terms, next.terms, addTerm, removeTerm)
}

// visitNameChanges visits added names before removed names, leaving common
// names in place.
func visitNameChanges[K comparable](prev, next []K, add, remove func(K)) {
	// Visit the new names, crossing off each old name next keeps.
	dropped := make(map[K]struct{}, len(prev))
	for _, name := range prev {
		dropped[name] = struct{}{}
	}
	for _, name := range next {
		if _, ok := dropped[name]; ok {
			delete(dropped, name)
			continue
		}
		add(name)
	}

	// Visit the old names next lacks.
	for name := range dropped {
		remove(name)
	}
}

// quadTerms returns the fields of a changed quad an index looks up.
func quadTerms(q GraphQuad) [4]quadTerm {
	return [4]quadTerm{
		{quadSubject, q.GetSubject()},
		{quadPredicate, q.GetPredicate()},
		{quadObj, q.GetObj()},
		{quadLabel, q.GetLabel()},
	}
}
