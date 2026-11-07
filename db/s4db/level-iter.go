package s4db

import (
	"bytes"
	"sort"
)

// levelIter iterates one level below 0 as a single merge layer. The level's
// runs are in key order and do not overlap, so it reads them one after
// another through one run layer at a time, and a scan over many runs of a
// level costs the merge one layer, not one per run.
type levelIter struct {
	// db reads the runs.
	db *DB
	// runs are the level's runs that may hold keys in range, in key order.
	runs []runMeta
	// i indexes the run cur reads.
	i int
	// cur reads runs[i]; nil before the first seek.
	cur *runIter
	// err is the first error.
	err error
}

// newLevelIter returns a layer over runs, a level's runs in key order.
func newLevelIter(db *DB, runs []runMeta) *levelIter {
	return &levelIter{db: db, runs: runs}
}

// valid reports whether the layer is on an entry.
func (l *levelIter) valid() bool {
	return l.err == nil && l.cur != nil && l.cur.valid()
}

// key returns the current key.
func (l *levelIter) key() []byte {
	return l.cur.key()
}

// entry returns the current entry.
func (l *levelIter) entry() ientry {
	return l.cur.entry()
}

// failure returns the first read or decode error.
func (l *levelIter) failure() error {
	return l.err
}

// seek positions at the first key at or after k, or the last at or before
// it when reverse, in the run that may hold it, moving on to the next run
// in the direction when that run holds none; nil selects the level's first
// or last key.
func (l *levelIter) seek(k []byte, reverse bool) {
	// Find the run: forward, the first ending at or after k; in reverse, the
	// last starting at or before it.
	l.i = 0
	switch {
	case reverse && k == nil:
		l.i = len(l.runs) - 1
	case reverse:
		l.i = sort.Search(len(l.runs), func(i int) bool { return bytes.Compare(l.runs[i].smallest, k) > 0 }) - 1
	case k != nil:
		l.i = sort.Search(len(l.runs), func(i int) bool { return bytes.Compare(l.runs[i].largest, k) >= 0 })
	}

	// Seek in it, then cross into the next runs until one holds a key.
	l.open()
	if l.cur != nil {
		l.cur.seek(k, reverse)
	}
	l.settle(reverse)
}

// step moves to the next key in the direction, crossing into the next run.
func (l *levelIter) step(reverse bool) {
	if !l.valid() {
		return
	}
	l.cur.step(reverse)
	l.settle(reverse)
}

// settle moves past runs the current one has run off the end of, positioned
// at their first or last key, until a run holds a key or none remain.
func (l *levelIter) settle(reverse bool) {
	for l.cur != nil && !l.cur.valid() {
		// Keep a run's error, which ends the layer.
		if err := l.cur.failure(); err != nil {
			l.err = err
			return
		}

		// Open the next run in the direction from its end.
		if reverse {
			l.i--
		} else {
			l.i++
		}
		l.open()
		if l.cur != nil {
			l.cur.seek(nil, reverse)
		}
	}
}

// open sets cur to a layer over runs[i], or nil when i is out of range.
func (l *levelIter) open() {
	if l.i < 0 || l.i >= len(l.runs) {
		l.cur = nil
		return
	}
	l.cur = &runIter{db: l.db, meta: l.runs[l.i], cache: true}
}

// _ is a type assertion
var _ layer = (*levelIter)(nil)
