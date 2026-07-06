//go:build goscript

package objecttypes

import (
	"context"

	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up a GoScript-supported object type by ID.
// Returns nil if not found.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	ot, err := lookupCoreObjectType(ctx, typeID)
	if ot != nil || err != nil {
		return ot, err
	}
	return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
}
