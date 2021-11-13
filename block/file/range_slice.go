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

// LocatePosition locates the range covering a position pos.
// returns the index of that range
// returns nil, 0, false if no range covering pos is located.
func (r RangeSlice) LocatePosition(pos int) (*Range, int, bool) {
	rlen := len(r)
	if rlen == 0 {
		return nil, 0, false
	}

	// find lowest index where start > pos
	// if not found, returns n
	idxAfter := sort.Search(rlen, func(i int) bool {
		return int(r[i].GetStart()) > pos
	})

	foundNonce, foundIdx := -1, -1
	// iterate backwards from that position
	// find range with highest nonce that is in range
	for i := idxAfter - 1; i >= 0; i-- {
		// check in range
		rng := r[i]
		rStart := int(rng.GetStart())
		rEnd := rStart + int(rng.GetLength())
		if pos < rStart || pos >= rEnd {
			continue
		}
		rNonce := int(rng.GetNonce())
		if rNonce < foundNonce {
			continue
		}
		foundNonce = rNonce
		foundIdx = i
	}
	if foundNonce == -1 {
		return nil, 0, false
	}
	return r[foundIdx], foundIdx, true
}

// _ is a type assertion
var _ sort.Interface = ((RangeSlice)(nil))
