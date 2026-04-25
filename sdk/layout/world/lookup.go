package s4wave_layout_world

import (
	"context"

	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// LookupObjectLayoutType looks up the ObjectLayout type by ID.
// Returns nil if the typeID does not match ObjectLayout.
func LookupObjectLayoutType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	if typeID == ObjectLayoutTypeID {
		return ObjectLayoutType, nil
	}
	return nil, nil
}
