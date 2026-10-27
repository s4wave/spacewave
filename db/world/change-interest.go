package world

// ChangeInterest is a ChangeFilter that also names the changes it may accept,
// so a change feed can index its watcher by those names and test it only on a
// change that names one of them. An interest without names, or one marked as
// needing every change, is tested on every change. A change feed reads the
// names once, when the watch starts, so a filter that may later accept other
// changes must start a new watch.
type ChangeInterest struct {
	// filter decides whether a change concerns the watcher.
	filter ChangeFilter
	// keys are the changed object keys the filter may accept.
	keys []string
	// terms are the changed quad fields the filter may accept.
	terms []quadTerm
	// all is set when the filter may accept a change naming none of them.
	all bool
	// shared are finished read sets whose changes the filter may accept. A
	// change feed names each set once for all the watchers sharing it, so a
	// watcher costs one entry per set, not one per key the set read.
	shared []*ReadSet
}

// quadField names one field of a graph quad.
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
// names none of them, so a change feed tests it on every change.
func NewChangeInterest(filter ChangeFilter) *ChangeInterest {
	return &ChangeInterest{filter: filter, all: true}
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
