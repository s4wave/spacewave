package objecttypes

import "slices"

// BuiltInObjectTypeIDs returns the compiled ObjectType table keys. The table
// is build-tag-specific, so runtime dispatch and inventory share one owner.
func BuiltInObjectTypeIDs() []string {
	ids := make([]string, 0, len(compiledObjectTypes))
	for typeID := range compiledObjectTypes {
		ids = append(ids, typeID)
	}
	slices.Sort(ids)
	return ids
}
