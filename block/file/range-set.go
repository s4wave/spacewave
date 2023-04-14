package file

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// rangeSet holds a set of ranges.
type rangeSet struct {
	v *[]*Range
}

// NewRangeSet builds a new range set container.
//
// bcs should be located at the range set sub-block.
func NewRangeSet(v *[]*Range, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&rangeSet{v: v}, bcs)
}

// IsNil checks if the range set is nil.
func (r *rangeSet) IsNil() bool {
	return r == nil
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *rangeSet) Get(i int) block.SubBlock {
	ranges := *r.v
	if len(ranges) > i {
		return ranges[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *rangeSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *rangeSet) Set(i int, ref block.SubBlock) {
	ranges := *r.v
	if i < 0 || i >= len(ranges) {
		return
	}
	ranges[i], _ = ref.(*Range)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *rangeSet) Truncate(nlen int) {
	ranges := *r.v
	olen := len(ranges)
	if nlen < 0 || nlen >= olen {
		return
	}
	for i := nlen; i < olen; i++ {
		ranges[i] = nil
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*rangeSet)(nil))
