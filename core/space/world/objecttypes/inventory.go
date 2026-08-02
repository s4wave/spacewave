package objecttypes

import "slices"

// BuiltInObjectTypeIDs returns the compiled ObjectType table keys. The table
// is build-tag-specific, so runtime dispatch and inventory read the same map.
func BuiltInObjectTypeIDs() []string {
	ids := make([]string, 0, len(compiledObjectTypes))
	for typeID := range compiledObjectTypes {
		ids = append(ids, typeID)
	}
	slices.Sort(ids)
	return ids
}
