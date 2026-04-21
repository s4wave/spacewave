package file

import (
	"sort"

	"github.com/s4wave/spacewave/db/block/sbset"
)

// HandleRangeSlice is a sortable slice of ranges.
type HandleRangeSlice struct {
	rangeSet *sbset.SubBlockSet
	ranges   *[]*Range
}

// NewHandleRangeSlice builds a slice with a handle for sorting the block graph.
func NewHandleRangeSlice(h *Handle) *HandleRangeSlice {
	ranges := &h.root.Ranges
	rangeSet := h.rangeSet
	return &HandleRangeSlice{rangeSet: rangeSet, ranges: ranges}
}

// Len is the number of elements in the collection.
func (r *HandleRangeSlice) Len() int {
	return r.rangeSet.Len()
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (r *HandleRangeSlice) Less(i, j int) bool {
	v := *r.ranges
	if i >= len(v) || j >= len(v) {
		return false
	}
	return v[i].LessThanRange(v[j])
}

// Swap swaps the elements with indexes i and j.
func (r HandleRangeSlice) Swap(i, j int) {
	r.rangeSet.Swap(i, j)
}

// _ is a type assertion
var _ sort.Interface = ((*HandleRangeSlice)(nil))
