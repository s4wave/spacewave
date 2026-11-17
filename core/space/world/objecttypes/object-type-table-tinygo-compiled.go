//go:build tinygo

package objecttypes

import s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"

var compiledObjectTypes = commonObjectTypes

// compiledObjectTypeMetadata is empty: no compiled type of this target has
// metadata.
var compiledObjectTypeMetadata map[string]*s4wave_objecttype_registry.ObjectTypeMetadata
