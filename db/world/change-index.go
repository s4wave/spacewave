package world

import (
	"iter"
	"maps"
	"slices"
)

// ChangeIndex holds a change feed's watchers by the names in their
// ChangeInterests, so a ChangeSet finds the watchers it may concern with one
// lookup per changed key and quad field, independent of the watcher count.
// Watchers whose interest names nothing are candidates for every change. A
// shared read set is named once while any watcher's interest holds it, so
// watchers sharing a large set subscribe and leave at the cost of their own
// reads. The caller synchronizes access.
type ChangeIndex[T comparable] struct {
	// interests maps each watcher to its interest.
	interests map[T]*ChangeInterest
	// watchers indexes the watchers by their own names.
	watchers nameIndex[T]
	// sets indexes the shared read sets the watchers hold by their names.
	sets nameIndex[*ReadSet]
	// shared maps each indexed shared set to its names and watchers.
	shared map[*ReadSet]*sharedSet[T]
}

// sharedSet is a shared read set the index names for its watchers.
type sharedSet[T comparable] struct {
	// names are the set's names, as indexed in sets.
	names *ChangeInterest
	// watchers are the watchers whose interest holds the set.
	watchers map[T]struct{}
}

// NewChangeIndex constructs an empty ChangeIndex.
func NewChangeIndex[T comparable]() *ChangeIndex[T] {
	return &ChangeIndex[T]{
		interests: make(map[T]*ChangeInterest),
		watchers:  newNameIndex[T](),
		sets:      newNameIndex[*ReadSet](),
		shared:    make(map[*ReadSet]*sharedSet[T]),
	}
}

// Len returns the number of watchers.
func (x *ChangeIndex[T]) Len() int {
	return len(x.interests)
}

// All iterates over the watchers.
func (x *ChangeIndex[T]) All() iter.Seq[T] {
	return maps.Keys(x.interests)
}

// Add indexes watcher w by the names in interest, replacing an earlier entry.
// A nil interest is tested on every change.
func (x *ChangeIndex[T]) Add(w T, interest *ChangeInterest) {
	// Replace an earlier entry, then index the watcher's own names.
	x.Remove(w)
	x.interests[w] = interest
	x.watchers.add(w, interest)
	if interest == nil || interest.all {
		return
	}

	// Hold each shared set, naming it when no watcher held it yet.
	for _, set := range interest.shared {
		entry := x.shared[set]
		if entry == nil {
			names := &ChangeInterest{}
			set.nameJoinedReads(names)
			entry = &sharedSet[T]{names: names, watchers: make(map[T]struct{})}
			x.shared[set] = entry
			x.sets.add(set, names)
		}
		entry.watchers[w] = struct{}{}
	}
}

// Remove drops watcher w from the index.
func (x *ChangeIndex[T]) Remove(w T) {
	// Find the names w was indexed by.
	interest, ok := x.interests[w]
	if !ok {
		return
	}
	delete(x.interests, w)
	x.watchers.remove(w, interest)
	if interest == nil || interest.all {
		return
	}

	// Release each shared set, and drop the names of a set no watcher holds.
	for _, set := range interest.shared {
		entry := x.shared[set]
		if entry == nil {
			continue
		}
		delete(entry.watchers, w)
		if len(entry.watchers) == 0 {
			x.sets.remove(set, entry.names)
			delete(x.shared, set)
		}
	}
}

// Candidates returns each watcher whose interest may accept changes once: the
// watchers named by a changed key or quad field, directly or through a shared
// set, and those tested on every change, or every watcher for an unknown
// ChangeSet. A watcher left out rejects changes.
func (x *ChangeIndex[T]) Candidates(changes *ChangeSet) []T {
	// An unknown ChangeSet reaches every watcher.
	if changes.Unknown {
		return slices.Collect(maps.Keys(x.interests))
	}

	// Gather the watchers the changes name, then those of each named set.
	found := make(map[T]struct{})
	x.watchers.collect(changes, found)
	sets := make(map[*ReadSet]struct{})
	x.sets.collect(changes, sets)
	for set := range sets {
		maps.Copy(found, x.shared[set].watchers)
	}
	return slices.Collect(maps.Keys(found))
}

// nameIndex holds entries by the object keys and quad fields their names
// list. Entries whose names are nil or marked as needing every change are
// collected for every change.
type nameIndex[T comparable] struct {
	// keys maps each named object key to its entries.
	keys map[string]map[T]struct{}
	// terms maps each named quad field to its entries.
	terms map[quadTerm]map[T]struct{}
	// all holds the entries collected for every change.
	all map[T]struct{}
}

// newNameIndex constructs an empty nameIndex.
func newNameIndex[T comparable]() nameIndex[T] {
	return nameIndex[T]{
		keys:  make(map[string]map[T]struct{}),
		terms: make(map[quadTerm]map[T]struct{}),
		all:   make(map[T]struct{}),
	}
}

// add indexes e by names.
func (x *nameIndex[T]) add(e T, names *ChangeInterest) {
	// An entry without names sees every change.
	if names == nil || names.all {
		x.all[e] = struct{}{}
		return
	}

	// Index the named keys and quad fields.
	for _, key := range names.keys {
		addIndexed(x.keys, key, e)
	}
	for _, term := range names.terms {
		addIndexed(x.terms, term, e)
	}
}

// remove drops e, indexed by names, and each name e alone held.
func (x *nameIndex[T]) remove(e T, names *ChangeInterest) {
	// An entry without names is held only in all.
	if names == nil || names.all {
		delete(x.all, e)
		return
	}

	// Drop e from each name.
	for _, key := range names.keys {
		removeIndexed(x.keys, key, e)
	}
	for _, term := range names.terms {
		removeIndexed(x.terms, term, e)
	}
}

// collect adds to found the entries named by a changed key or quad field and
// those collected for every change.
func (x *nameIndex[T]) collect(changes *ChangeSet, found map[T]struct{}) {
	maps.Copy(found, x.all)
	for _, key := range changes.Keys {
		maps.Copy(found, x.keys[key])
	}
	for _, q := range changes.Quads {
		for _, term := range quadTerms(q) {
			maps.Copy(found, x.terms[term])
		}
	}
}

// addIndexed adds w to the watchers of name.
func addIndexed[K comparable, T comparable](index map[K]map[T]struct{}, name K, w T) {
	set := index[name]
	if set == nil {
		set = make(map[T]struct{})
		index[name] = set
	}
	set[w] = struct{}{}
}

// removeIndexed removes w from the watchers of name, and name once empty.
func removeIndexed[K comparable, T comparable](index map[K]map[T]struct{}, name K, w T) {
	set := index[name]
	delete(set, w)
	if len(set) == 0 {
		delete(index, name)
	}
}
