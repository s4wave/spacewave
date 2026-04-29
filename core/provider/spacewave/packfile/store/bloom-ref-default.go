//go:build !tinygo

package store

import (
	"weak"

	bbloom "github.com/bits-and-blooms/bloom/v3"
)

// bloomRef is a weak reference to a bloom filter, allowing the GC to
// reclaim cached filters under memory pressure when no live caller
// retains them.
type bloomRef struct {
	wp weak.Pointer[bbloom.BloomFilter]
}

// makeBloomRef wraps bf in a weak pointer.
func makeBloomRef(bf *bbloom.BloomFilter) bloomRef {
	return bloomRef{wp: weak.Make(bf)}
}

// Value returns the underlying bloom filter, or nil if it has been
// reclaimed by the GC.
func (r bloomRef) Value() *bbloom.BloomFilter {
	return r.wp.Value()
}
