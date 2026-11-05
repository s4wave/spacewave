package s4db

import "sync/atomic"

// Flow is the length of the values commits stored and freed under one key
// family since the database opened.
type Flow struct {
	// Written is the length of the values commits stored outside the index.
	Written int64
	// Freed is the length of the stored values commits replaced or deleted.
	Freed int64
}

// flowCount counts a family's flow with atomics, so Stats takes no lock.
type flowCount struct {
	// written and freed are the family's Flow.
	written, freed atomic.Int64
}
