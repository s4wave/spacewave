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

// ComparableNamedSubBlock is a NamedSubBlock that has an Equals function.
type ComparableNamedSubBlock interface {
	NamedSubBlock

	// Equals compares the block to the other block for equality.
	Equals(ot ComparableNamedSubBlock) bool
}

// CompareNamedSubBlocks compares two sets of ComparableNamedSubBlock.
// Returns the added, removed, and changed values.
func CompareNamedSubBlocks[T ComparableNamedSubBlock](a, b []T) (added, removed, changed []T) {
	aVals := make(map[string]T)
	for _, val := range a {
		aVals[val.GetName()] = val
	}

	for _, val := range b {
		valName := val.GetName()
		aVal, aValOk := aVals[valName]
		if !aValOk {
			added = append(added, val)
		} else {
			if !aVal.Equals(val) {
				changed = append(changed, val)
			}
			delete(aVals, valName)
		}
	}

	for _, val := range aVals {
		removed = append(removed, val)
	}

	return added, removed, changed
}
