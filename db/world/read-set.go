package world

import (
	"slices"
	"strings"
	"sync"
)

// ReadSet records what a view read from the World, so a ChangeSet can be
// checked against it. Reads it cannot describe precisely mark it conservative,
// and any change touches it. A ReadSet can track several WorldStates at once.
type ReadSet struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// keys are the object keys read, including keys that were not found.
	keys map[string]struct{}
	// prefixes are the object key prefixes iterated.
	prefixes []string
	// patterns are the graph quad lookup filters.
	patterns []graphPattern
	// graph is set when the graph was read beyond quad lookups.
	graph bool
	// all is set when the World was read beyond objects and the graph.
	all bool
}

// NewReadSet constructs an empty ReadSet.
func NewReadSet() *ReadSet {
	return &ReadSet{keys: make(map[string]struct{})}
}

// Track returns a WorldState that records the reads made through it.
// The seqno of ws identifies the state the reads observed.
func (r *ReadSet) Track(ws WorldState) WorldState {
	return &readSetWorldState{WorldState: ws, reads: r}
}

// Touched checks whether a ChangeSet may change what was read.
func (r *ReadSet) Touched(changes *ChangeSet) bool {
	// Hold the reads while comparing.
	r.mtx.Lock()
	defer r.mtx.Unlock()

	// Unknown and conservative reads match every non-empty change.
	if changes.Unknown {
		return true
	}
	if len(changes.Keys) == 0 && len(changes.Quads) == 0 {
		return false
	}
	if r.all {
		return true
	}

	// A changed key matches a read key, a read prefix, or a pattern endpoint.
	if slices.ContainsFunc(changes.Keys, r.touchesKey) {
		return true
	}

	// A changed quad matches a graph read.
	for _, q := range changes.Quads {
		if r.graph {
			return true
		}
		for _, p := range r.patterns {
			if p.matches(q) {
				return true
			}
		}
	}
	return false
}

// touchesKey checks a changed object key. The caller holds mtx.
func (r *ReadSet) touchesKey(key string) bool {
	// Match the key itself and the iterated prefixes.
	if _, ok := r.keys[key]; ok {
		return true
	}
	for _, prefix := range r.prefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}

	// Match a graph read naming the key as a subject or object.
	if r.graph {
		return true
	}
	value := KeyToGraphValue(key).String()
	for _, p := range r.patterns {
		if p.subject == value || p.obj == value {
			return true
		}
	}
	return false
}

// addKeys records object key reads.
func (r *ReadSet) addKeys(keys ...string) {
	r.mtx.Lock()
	for _, key := range keys {
		r.keys[key] = struct{}{}
	}
	r.mtx.Unlock()
}

// addPrefix records an object key prefix read.
func (r *ReadSet) addPrefix(prefix string) {
	r.mtx.Lock()
	r.prefixes = append(r.prefixes, prefix)
	r.mtx.Unlock()
}

// addPatterns records graph quad lookups.
func (r *ReadSet) addPatterns(filters ...GraphQuad) {
	r.mtx.Lock()
	for _, q := range filters {
		r.patterns = append(r.patterns, graphPattern{
			subject:   q.GetSubject(),
			predicate: q.GetPredicate(),
			obj:       q.GetObj(),
			label:     q.GetLabel(),
		})
	}
	r.mtx.Unlock()
}

// markGraph records a graph read that quad patterns cannot describe.
func (r *ReadSet) markGraph() {
	r.mtx.Lock()
	r.graph = true
	r.mtx.Unlock()
}

// markAll records a read that object keys and graph reads cannot describe.
func (r *ReadSet) markAll() {
	r.mtx.Lock()
	r.all = true
	r.mtx.Unlock()
}
