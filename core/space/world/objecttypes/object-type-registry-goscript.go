//go:build goscript

package objecttypes

import (
	"context"

	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up a GoScript-supported object type by ID.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	if objectType := compiledObjectTypes[typeID]; objectType != nil {
		return objectType, nil
	}
	return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
}
