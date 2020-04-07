package file

import "sort"

// RangeSlice is a sortable slice of ranges.
type RangeSlice []*Range

// Len is the number of elements in the collection.
func (r RangeSlice) Len() int {
	return len(r)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (r RangeSlice) Less(i, j int) bool {
	return r[i].LessThanRange(r[j])
}

// Swap swaps the elements with indexes i and j.
func (r RangeSlice) Swap(i, j int) {
	jx := r[j]
	r[j] = r[i]
	r[i] = jx
}

// _ is a type assertion
var _ sort.Interface = ((RangeSlice)(nil))
