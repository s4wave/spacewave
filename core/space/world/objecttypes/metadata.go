package objecttypes

import s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"

// LookupObjectTypeMetadata returns a copy of a compiled object type's
// metadata, or nil when the type has none. Plugin types register theirs with
// the object type registry instead.
func LookupObjectTypeMetadata(typeID string) *s4wave_objecttype_registry.ObjectTypeMetadata {
	return compiledObjectTypeMetadata[typeID].CloneVT()
}
