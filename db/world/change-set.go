package world

import "slices"

// ChangeSet describes the World changes between two sequence numbers.
type ChangeSet struct {
	// Unknown means the changes could not be determined.
	// Every ReadSet is touched by an unknown ChangeSet.
	Unknown bool
	// Keys are the object keys that were set, deleted, renamed, or revised.
	// A rename lists both the old and the new key. A revision that only
	// follows a set or deleted quad referencing the object is reported
	// through Quads alone.
	Keys []string
	// Quads are the graph quads that were set or deleted, each field spelled
	// as the graph stores it.
	Quads []GraphQuad
}

// NewUnknownChangeSet returns a ChangeSet whose changes are unknown.
func NewUnknownChangeSet() *ChangeSet {
	return &ChangeSet{Unknown: true}
}

// MergeChangeSets returns the union of two ChangeSets without modifying
// either, sharing b when a is nil. The union is unknown when either is, or
// when it lists more than maxEntries keys and quads.
func MergeChangeSets(a, b *ChangeSet, maxEntries int) *ChangeSet {
	// Share a lone ChangeSet and keep unknown sets unknown.
	if a == nil {
		return b
	}
	if a.Unknown || b.Unknown {
		return NewUnknownChangeSet()
	}

	// Join the keys without duplicates, and the quads.
	keys := slices.Concat(a.Keys, b.Keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	quads := slices.Concat(a.Quads, b.Quads)
	if len(keys)+len(quads) > maxEntries {
		return NewUnknownChangeSet()
	}
	return &ChangeSet{Keys: keys, Quads: quads}
}
