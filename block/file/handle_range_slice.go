package file

import "sort"

// HandleRangeSlice is a sortable slice of ranges.
type HandleRangeSlice struct {
	h      *Handle
	ranges []*Range
}

// NewHandleRangeSlice builds a slice with a handle for sorting the block graph.
func NewHandleRangeSlice(h *Handle) *HandleRangeSlice {
	return &HandleRangeSlice{h: h, ranges: h.root.GetRanges()}
}

// Len is the number of elements in the collection.
func (r *HandleRangeSlice) Len() int {
	return len(r.ranges)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (r *HandleRangeSlice) Less(i, j int) bool {
	return r.ranges[i].LessThanRange(r.ranges[j])
}

// Swap swaps the elements with indexes i and j.
func (r HandleRangeSlice) Swap(i, j int) {
	iRefID := NewFileRangeRefId(i)
	jRefID := NewFileRangeRefId(j)

	ics := r.h.bcs.FollowRef(iRefID, r.ranges[i].GetRef())
	jcs := r.h.bcs.FollowRef(jRefID, r.ranges[j].GetRef())

	jx := r.ranges[j]
	r.ranges[j] = r.ranges[i]
	r.ranges[i] = jx

	r.h.bcs.SetRef(iRefID, jcs)
	r.h.bcs.SetRef(jRefID, ics)
}

// _ is a type assertion
var _ sort.Interface = ((*HandleRangeSlice)(nil))
