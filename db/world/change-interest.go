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

// quadTerms returns the fields of a changed quad an index looks up.
func quadTerms(q GraphQuad) [4]quadTerm {
	return [4]quadTerm{
		{quadSubject, q.GetSubject()},
		{quadPredicate, q.GetPredicate()},
		{quadObj, q.GetObj()},
		{quadLabel, q.GetLabel()},
	}
}
