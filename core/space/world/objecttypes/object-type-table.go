package objecttypes

import (
	"maps"

	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// extendObjectTypes creates the immutable compiled table once during package
// initialization. No lookup allocates or mutates the table.
func extendObjectTypes(base, extension map[string]objecttype.ObjectType) map[string]objecttype.ObjectType {
	out := make(map[string]objecttype.ObjectType, len(base)+len(extension))
	maps.Copy(out, base)
	maps.Copy(out, extension)
	return out
}
