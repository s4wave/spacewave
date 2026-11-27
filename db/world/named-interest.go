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
	filter := func(changes *ChangeSet) bool {
		if changes.Unknown {
			return true
		}
		for _, key := range changes.Keys {
			if _, ok := keySet[key]; ok {
				return true
			}
		}
		for _, q := range changes.Quads {
			for _, term := range quadTerms(q) {
				for _, p := range byTerm[term] {
					if p.matches(q) {
						return true
					}
				}
			}
		}
		return false
	}
	return &ChangeInterest{filter: filter, keys: named, terms: terms}, nil
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
