//go:build !tinygo

package store

import (
	"weak"

	"github.com/s4wave/spacewave/db/block/bloom"
)

// bloomRef is a weak reference to a bloom filter, allowing the GC to
// reclaim cached filters under memory pressure when no live caller
// retains them.
type bloomRef struct {
	wp weak.Pointer[bloom.Filter]
}

// makeBloomRef wraps bf in a weak pointer.
func makeBloomRef(bf *bloom.Filter) bloomRef {
	return bloomRef{wp: weak.Make(bf)}
}

// Value returns the underlying bloom filter, or nil if it has been
// reclaimed by the GC.
func (r bloomRef) Value() *bloom.Filter {
	return r.wp.Value()
}
