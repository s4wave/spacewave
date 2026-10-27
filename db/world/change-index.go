package world

import (
	"iter"
	"maps"
	"slices"
)

// ChangeIndex holds a change feed's watchers by the names in their
// ChangeInterests, so a ChangeSet finds the watchers it may concern with one
// lookup per changed key and quad field, independent of the watcher count.
// Watchers whose interest names nothing are candidates for every change.
//
// A shared read set is indexed once by its own reads while a watcher or a held
// set that joined it holds it, and links to the held sets it joined. A change
// a set's reads name reaches the set's watchers and those of every held set
// that joined it, directly or through others. A watcher holding a set built
// from large ones thus subscribes and leaves at the cost of its own reads and
// the sets it joins, never of every key below them. The caller synchronizes
// access.
type ChangeIndex[T comparable] struct {
	// interests maps each watcher to its interest.
	interests map[T]*ChangeInterest
	// watchers indexes the watchers by their own names.
	watchers nameIndex[T]
	// sets indexes the held read sets by their own names.
	sets nameIndex[*heldSet[T]]
	// held maps each held read set to its entry.
	held map[*ReadSet]*heldSet[T]
}

// heldSet is a read set the index names while a watcher or another held set
// holds it. Its names freeze when it is first held: a shared set is finished.
type heldSet[T comparable] struct {
	// set is the read set.
	set *ReadSet
	// names are the set's own reads, as indexed in sets.
	names *ChangeInterest
	// joined are the held sets this set joined. A set whose own names need
	// every change holds none: it is a candidate for every change already.
	joined []*heldSet[T]
	// parents are the held sets that joined this one.
	parents map[*heldSet[T]]struct{}
	// watchers are the watchers whose interest holds the set.
	watchers map[T]struct{}
}

// NewChangeIndex constructs an empty ChangeIndex.
func NewChangeIndex[T comparable]() *ChangeIndex[T] {
	return &ChangeIndex[T]{
		interests: make(map[T]*ChangeInterest),
		watchers:  newNameIndex[T](),
		sets:      newNameIndex[*heldSet[T]](),
		held:      make(map[*ReadSet]*heldSet[T]),
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

	// Hold each shared set for w.
	for _, set := range interest.shared {
		x.hold(set).watchers[w] = struct{}{}
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

	// Release each shared set w held.
	for _, set := range interest.shared {
		if entry := x.held[set]; entry != nil {
			delete(entry.watchers, w)
			x.releaseUnheld(entry)
		}
	}
}

// Candidates returns each watcher whose interest may accept changes once: the
// watchers named by a changed key or quad field, directly or through a held
// set, and those tested on every change, or every watcher for an unknown
// ChangeSet. A watcher left out rejects changes.
func (x *ChangeIndex[T]) Candidates(changes *ChangeSet) []T {
	// An unknown ChangeSet reaches every watcher.
	if changes.Unknown {
		return slices.Collect(maps.Keys(x.interests))
	}

	// Gather the watchers the changes name, and the held sets they name.
	found := make(map[T]struct{})
	x.watchers.collect(changes, found)
	named := make(map[*heldSet[T]]struct{})
	x.sets.collect(changes, named)

	// Add the watchers of each named set and of every set that joined one.
	pending := slices.Collect(maps.Keys(named))
	for len(pending) != 0 {
		entry := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		maps.Copy(found, entry.watchers)
		for parent := range entry.parents {
			if _, ok := named[parent]; !ok {
				named[parent] = struct{}{}
				pending = append(pending, parent)
			}
		}
	}
	return slices.Collect(maps.Keys(found))
}

// hold returns the entry of set, indexing it by its own reads and holding
// the sets it joined when no watcher or set held it yet.
func (x *ChangeIndex[T]) hold(set *ReadSet) *heldSet[T] {
	// Reuse the entry of a held set.
	if entry := x.held[set]; entry != nil {
		return entry
	}

	// Index the set by its own reads.
	names := &ChangeInterest{}
	joined := set.nameReads(names)
	entry := &heldSet[T]{
		set:      set,
		names:    names,
		parents:  make(map[*heldSet[T]]struct{}),
		watchers: make(map[T]struct{}),
	}
	x.held[set] = entry
	x.sets.add(entry, names)
	if names.all {
		return entry
	}

	// Hold the sets it joined, each linked back to it.
	for _, other := range joined {
		child := x.hold(other)
		child.parents[entry] = struct{}{}
		entry.joined = append(entry.joined, child)
	}
	return entry
}

// releaseUnheld drops entry once no watcher or set holds it, and releases the
// sets it joined.
func (x *ChangeIndex[T]) releaseUnheld(entry *heldSet[T]) {
	// Keep a set still held.
	if len(entry.watchers) != 0 || len(entry.parents) != 0 {
		return
	}

	// Drop its names, then its hold on each set it joined.
	x.sets.remove(entry, entry.names)
	delete(x.held, entry.set)
	for _, child := range entry.joined {
		delete(child.parents, entry)
		x.releaseUnheld(child)
	}
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
