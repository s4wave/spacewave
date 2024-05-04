package merge

import (
	"cmp"
	"slices"
)

// MergeAndSortSlices merges the slices and sorts them iif dirty.
func MergeAndSortSlices[S ~[]E, E cmp.Ordered](mergeTo *S, mergeFrom S) {
	var dirty bool
	dest := *mergeTo
	for _, value := range mergeFrom {
		var zero E
		if value != zero && !slices.Contains(dest, value) {
			dirty = true
			dest = append(dest, value)
		}
	}
	if dirty {
		slices.Sort(dest)
		*mergeTo = dest
	}
}
