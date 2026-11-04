package viewcache

import (
	"context"
	"sync"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// revisionEntry is one value computed, or being computed, at revision at.
type revisionEntry struct {
	// name is the key the value is kept under.
	name string
	// done is closed once the fields below are set.
	done chan struct{}
	// at is the revision the value was computed from.
	at    uint64
	value any
	// reads records what computing value read itself. The values it reused
	// keep their own reads, in parts, so a value built from large parts
	// shares their sets instead of copying them.
	reads *world.ReadSet
	// blocks holds the blocks computing value read itself, so eviction
	// keeps them on the disk while the value is kept or a live view built
	// from it keeps the hold.
	blocks *block.RefHold
	// wake is the earliest time the value asked to rerun at, if any.
	wake time.Time
	err  error
	// stale is set before done closes when a change the cache passed while
	// the value computed touched its reads: the value then holds only at
	// revision at.
	stale bool
	// parts are the cached values computing value reused, whose reads are
	// part of value's. Set before done closes.
	parts []*revisionEntry
	// facets are the cached values computing value read through a facet
	// instead: they leave value alone while the sub-keys it read are
	// unchanged. Set before done closes; a catch-up replaces each part
	// under the cache's mtx.
	facets []facetRead
	// finished is set, under the cache's mtx, once done closes and the value
	// is checked against the changes passed while it computed.
	finished bool
	// Recent is set, under the cache's mtx, once recent answered the
	// finished value: a catch-up that drops it keeps it as the previous
	// value.
	recent bool
	// merged joins reads and every part's into one set for the views that
	// depend on the value, built once when the first one asks.
	merged     *world.ReadSet
	mergedOnce sync.Once
	// checked joins reads and every part's checked set into the one set the
	// cache indexes a value by when it reaches no part read by facet, built
	// once when the cache first indexes such a value or one built from it.
	// A facet part is replaced on a catch-up, so a value reaching one has no
	// fixed set and lists its sets each time it is indexed.
	checked     *world.ReadSet
	facetFree   bool
	checkedOnce sync.Once
}

// expired reports whether the time the value asked to rerun at has passed.
// A value without one reads no clock.
func (e *revisionEntry) expired() bool {
	return !e.wake.IsZero() && !time.Now().Before(e.wake)
}

// touched reports whether changes may alter the value: whether they touch
// its own reads or any part's. Each ReadSet remembers its answer for the last
// ChangeSet, so the parts many values share are checked once per change.
// next holds the values a catch-up already computed again in place of facet
// parts it touched; a facet part in next counts as touching the value only
// when its facet changed, and one not in it counts as a plain part. The
// caller holds the cache's mtx.
func (e *revisionEntry) touched(changes *world.ChangeSet, next map[*revisionEntry]*revisionEntry) bool {
	if e.reads.Touched(changes) {
		return true
	}
	for _, part := range e.parts {
		if part.touched(changes, next) {
			return true
		}
	}
	for _, facet := range e.facets {
		if replaced := next[facet.part]; replaced != nil {
			if facet.changed(facet.part.value, replaced.value) {
				return true
			}
		} else if facet.part.touched(changes, next) {
			return true
		}
	}
	return false
}

// facetRead is a cached value read through a facet: only the sub-keys of the
// part its reader named, not all of it.
type facetRead struct {
	// part is the cached value, the newest the cache holds for its name.
	part *revisionEntry
	// compute computes the part again.
	compute func(context.Context, world.WorldState) (any, error)
	// changed reports whether the sub-keys the reader named differ between
	// two values of the part.
	changed func(previous, next any) bool
	// reads are the part's merged reads when the value read it, kept for
	// the views that depend on the value, which wake on every change to the
	// part and find its result.
	reads *world.ReadSet
}

// viewReads returns one read set holding the value's reads and every part's,
// for a view. A view adding each part's set instead would pay a linear
// dedupe per set in its evaluation, and the many values a view reads share
// their merged sets. The set joins each part's merged set by reference, so
// building it costs one join per direct part, not one per set below it. Call
// it once the value is done.
func (e *revisionEntry) viewReads() *world.ReadSet {
	e.mergedOnce.Do(func() {
		// Reuse direct reads or join the immutable reads of each dependency.
		if len(e.parts) == 0 && len(e.facets) == 0 {
			e.merged = e.reads
			return
		}
		sets := make([]*world.ReadSet, 0, 1+len(e.parts)+len(e.facets))
		sets = append(sets, e.reads)
		for _, part := range e.parts {
			sets = append(sets, part.viewReads())
		}
		for _, facet := range e.facets {
			sets = append(sets, facet.reads)
		}
		e.merged = joinedReads(sets)
	})
	return e.merged
}

// joinedReads returns a new read set that joins sets by reference.
func joinedReads(sets []*world.ReadSet) *world.ReadSet {
	// Join the supplied read sets through one view evaluation.
	joined := world.NewReadSet()
	child, finish := world.WithViewReads(context.Background(), joined)
	for _, set := range sets {
		world.AddViewReads(child, set, time.Time{})
	}
	finish()
	return joined
}

// checkedSet returns the one set the cache indexes a value reaching no part
// read by facet by, and whether the value reaches none. The set joins the
// value's reads and each part's checked set by reference, so the many values
// built from a shared part hold it once in the index, and indexing a value
// costs one entry per direct part. Call it once the value is done.
func (e *revisionEntry) checkedSet() (*world.ReadSet, bool) {
	e.checkedOnce.Do(func() {
		// Build one indexed set only when no dependency reaches a facet.
		if len(e.facets) != 0 {
			return
		}
		sets := make([]*world.ReadSet, 0, 1+len(e.parts))
		sets = append(sets, e.reads)
		for _, part := range e.parts {
			set, free := part.checkedSet()
			if !free {
				return
			}
			sets = append(sets, set)
		}
		e.facetFree = true
		if len(e.parts) == 0 {
			e.checked = e.reads
			return
		}
		e.checked = joinedReads(sets)
	})
	return e.checked, e.facetFree
}

// checkedSets lists the sets touched consults for the value: its own reads and
// those of every part it reused and every part it reads by facet,
// transitively, each part once. A part reaching no facet part counts as its
// one checked set.
func (e *revisionEntry) checkedSets() []*world.ReadSet {
	// Walk each dependency once, using joined sets for facet-free branches.
	var sets []*world.ReadSet
	seen := make(map[*revisionEntry]bool)
	stack := []*revisionEntry{e}
	for len(stack) != 0 {
		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[next] {
			continue
		}
		seen[next] = true
		if set, free := next.checkedSet(); free {
			sets = append(sets, set)
			continue
		}
		sets = append(sets, next.reads)
		stack = append(stack, next.parts...)
		for _, facet := range next.facets {
			stack = append(stack, facet.part)
		}
	}
	return sets
}
