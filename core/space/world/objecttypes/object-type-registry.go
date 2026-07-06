//go:build !tinygo && !goscript

package objecttypes

import (
	"context"

	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	s4wave_forge_world "github.com/s4wave/spacewave/sdk/forge/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_org_world "github.com/s4wave/spacewave/sdk/org/world"
	s4wave_secret_world "github.com/s4wave/spacewave/sdk/secret/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up an object type by ID.
// Returns nil if not found.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	ot, err := lookupCoreObjectType(ctx, typeID)
	if ot != nil || err != nil {
		return ot, err
	}
	switch typeID {
	case s4wave_vm.VmV86TypeID:
		return s4wave_vm_world.VmV86Type, nil
	case s4wave_vm.V86ImageTypeID:
		return s4wave_vm_world.V86ImageType, nil
	case s4wave_org.OrganizationTypeID:
		return s4wave_org_world.OrganizationType, nil
	case s4wave_secret_world.SecretTypeID:
		return s4wave_secret_world.SecretType, nil
	case bldr_manifest_world.ManifestTypeID:
		return objecttype.NewObjectType(bldr_manifest_world.ManifestTypeID, s4wave_forge_world.ForgeReadOnlyFactory), nil
	default:
		return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
	}
}
