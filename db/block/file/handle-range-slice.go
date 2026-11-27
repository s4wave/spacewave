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

// sortRanges sorts the handle's ranges together with their block graph.
func (r *Handle) sortRanges() {
	sort.Sort(&HandleRangeSlice{rangeSet: r.rangeSet, ranges: &r.root.Ranges})
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
var _ sort.Interface = (*HandleRangeSlice)(nil)
