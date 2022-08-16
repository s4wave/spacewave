package block

import "sort"

// SortNamedSubBlocks sorts a set of NamedSubBlock.
func SortNamedSubBlocks[T NamedSubBlock](namedSubBlocks []T) {
	sort.Slice(namedSubBlocks, func(i, j int) bool {
		return namedSubBlocks[i].GetName() < namedSubBlocks[j].GetName()
	})
}

// IsNamedSubBlocksSorted checks if the set of NamedSubBlock is sorted.
func IsNamedSubBlocksSorted[T NamedSubBlock](namedSubBlocks []T) bool {
	return sort.SliceIsSorted(namedSubBlocks, func(i, j int) bool {
		return namedSubBlocks[i].GetName() < namedSubBlocks[j].GetName()
	})
}
