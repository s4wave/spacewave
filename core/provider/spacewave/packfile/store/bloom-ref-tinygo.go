//go:build tinygo

package store

import (
	"github.com/s4wave/spacewave/db/block/bloom"
)

// bloomRef is a strong reference to a bloom filter.
//
// TinyGo's linker is missing weak.runtime_makeStrongFromWeak, so this
// build retains cached filters strongly. Filters live as long as the
// store's bloom map entry.
type bloomRef struct {
	bf *bloom.Filter
}

// makeBloomRef wraps bf in a strong reference.
func makeBloomRef(bf *bloom.Filter) bloomRef {
	return bloomRef{bf: bf}
}

// Value returns the underlying bloom filter.
func (r bloomRef) Value() *bloom.Filter {
	return r.bf
}
