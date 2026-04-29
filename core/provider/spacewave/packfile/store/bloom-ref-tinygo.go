//go:build tinygo

package store

import (
	bbloom "github.com/bits-and-blooms/bloom/v3"
)

// bloomRef is a strong reference to a bloom filter.
//
// TinyGo's linker is missing weak.runtime_makeStrongFromWeak, so this
// build retains cached filters strongly. Filters live as long as the
// store's bloom map entry.
type bloomRef struct {
	bf *bbloom.BloomFilter
}

// makeBloomRef wraps bf in a strong reference.
func makeBloomRef(bf *bbloom.BloomFilter) bloomRef {
	return bloomRef{bf: bf}
}

// Value returns the underlying bloom filter.
func (r bloomRef) Value() *bbloom.BloomFilter {
	return r.bf
}
