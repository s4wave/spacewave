package world

import (
	"errors"
	"slices"
)

// ErrGraphPatternNamesNoField is returned for a graph pattern that sets no
// field: the pattern matches every quad, so no index could select the changes
// it accepts.
var ErrGraphPatternNamesNoField = errors.New("graph pattern names no field")

// NewNamedInterest returns an interest in the changes to any of keys and the
// changes that set or delete a quad matching any of patterns. An empty field
// of a pattern matches any value. A change feed indexes the interest by each
// key and by one field of each pattern, so a change referencing none of them
// never tests the interest, and a link added from an origin reaches a pattern
// matching that origin without the interest selecting the linked object in
// advance. With no keys and no patterns, the interest accepts only an unknown
// change.
func NewNamedInterest(keys []string, patterns []GraphQuad) (*ChangeInterest, error) {
	// Index the patterns by the field the change feed indexes them by.
	byTerm := make(map[quadTerm][]graphPattern, len(patterns))
	terms := make([]quadTerm, 0, len(patterns))
	for _, filter := range patterns {
		p := newGraphPattern(filter)
		term, ok := p.term()
		if !ok {
			return nil, ErrGraphPatternNamesNoField
		}
		if _, seen := byTerm[term]; !seen {
			terms = append(terms, term)
		}
		byTerm[term] = append(byTerm[term], p)
	}

	// Record each key once.
	keySet := make(map[string]struct{}, len(keys))
	named := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, seen := keySet[key]; !seen {
			keySet[key] = struct{}{}
			named = append(named, key)
		}
	}

	// Accept an unknown change, a changed key, or a changed quad matching a
	// pattern filed under one of its fields.
	sel := &namedSelect{keys: keySet, byTerm: byTerm}
	return &ChangeInterest{filter: sel.accepts, keys: named, terms: terms, named: sel}, nil
}

// namedSelect matches the changes a named interest selects: a changed key it
// names, and a changed quad matching a pattern filed under one of the quad's
// fields.
type namedSelect struct {
	// keys are the named object keys.
	keys map[string]struct{}
	// byTerm files each pattern under the field the change feed indexes it by.
	byTerm map[quadTerm][]graphPattern
}

// accepts reports whether changes is unknown or holds a selected entry.
func (n *namedSelect) accepts(changes *ChangeSet) bool {
	return changes.Unknown ||
		slices.ContainsFunc(changes.Keys, n.key) ||
		slices.ContainsFunc(changes.Quads, n.quad)
}

// key reports whether the interest names key.
func (n *namedSelect) key(key string) bool {
	_, ok := n.keys[key]
	return ok
}

// quad reports whether q matches a pattern of the interest.
func (n *namedSelect) quad(q GraphQuad) bool {
	for _, term := range quadTerms(q) {
		for _, p := range n.byTerm[term] {
			if p.matches(q) {
				return true
			}
		}
	}
	return false
}

// Select returns the part of changes the interest selects. A feed merges
// the changes of several revisions into one ChangeSet and tests the union,
// so an accepted union may carry entries for names the interest does not
// hold. A named interest drops them, returning changes itself when it
// selects every entry. Any other interest, and an unknown ChangeSet, return
// changes unchanged.
func (c *ChangeInterest) Select(changes *ChangeSet) *ChangeSet {
	// Only a named interest can tell which entries it selects.
	if c == nil || c.named == nil || changes == nil || changes.Unknown {
		return changes
	}

	// Share changes when the interest selects every entry.
	otherKey := func(k string) bool { return !c.named.key(k) }
	otherQuad := func(q GraphQuad) bool { return !c.named.quad(q) }
	if !slices.ContainsFunc(changes.Keys, otherKey) && !slices.ContainsFunc(changes.Quads, otherQuad) {
		return changes
	}

	// Copy out the selected entries, leaving the shared ChangeSet intact.
	return &ChangeSet{
		Keys:  slices.DeleteFunc(slices.Clone(changes.Keys), otherKey),
		Quads: slices.DeleteFunc(slices.Clone(changes.Quads), otherQuad),
	}
}

// Names returns object keys and graph patterns that together select every
// change the interest may accept, for a change feed in another process to
// index it by: NewNamedInterest(keys, patterns) accepts at least what the
// interest accepts. A pattern sets one field, so it may match changes the
// interest rejects. The caller still filters with the interest. The names of
// the shared sets and of every set they joined are included. ok is false when
// the interest may accept a change referencing none of them, such as one built
// by NewChangeInterest or from a set that read a prefix.
func (c *ChangeInterest) Names() (keys []string, patterns []GraphQuad, ok bool) {
	// An interest in every change has no names.
	if c == nil || c.all {
		return nil, nil, false
	}

	// Collect the interest's own names, then each shared set's, walking the
	// sets they joined once each.
	names := &ChangeInterest{keys: slices.Clone(c.keys), terms: slices.Clone(c.terms)}
	pending := slices.Clone(c.shared)
	seen := make(map[*ReadSet]struct{}, len(pending))
	for len(pending) != 0 {
		set := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, ok := seen[set]; ok {
			continue
		}
		seen[set] = struct{}{}
		pending = append(pending, set.nameReads(names)...)
		if names.all {
			return nil, nil, false
		}
	}

	// Spell each term as a pattern setting only its field.
	patterns = make([]GraphQuad, len(names.terms))
	for i, term := range names.terms {
		patterns[i] = term.pattern()
	}
	return names.keys, patterns, true
}

// pattern returns the graph pattern setting only the term's field. The
// pattern selects changes for a change feed. The pattern is not a lookup.
func (t quadTerm) pattern() GraphQuad {
	var fields [4]string
	fields[t.field] = t.value
	return NewGraphQuad(fields[0], fields[1], fields[2], fields[3])
}
