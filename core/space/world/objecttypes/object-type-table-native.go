//go:build !tinygo && !goscript

package objecttypes

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_forge_world "github.com/s4wave/spacewave/sdk/forge/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_org_world "github.com/s4wave/spacewave/sdk/org/world"
	s4wave_secret_world "github.com/s4wave/spacewave/sdk/secret/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

const spaceSettingsTypeID = "github.com/s4wave/spacewave/core/space/world.SpaceSettings"

var compiledObjectTypes = extendObjectTypes(commonObjectTypes, map[string]objecttype.ObjectType{
	spaceSettingsTypeID:                objecttype.NewObjectType(spaceSettingsTypeID, spaceSettingsReadOnlyFactory),
	s4wave_vm.VmV86TypeID:              s4wave_vm_world.VmV86Type,
	s4wave_vm.V86ImageTypeID:           s4wave_vm_world.V86ImageType,
	s4wave_org.OrganizationTypeID:      s4wave_org_world.OrganizationType,
	s4wave_secret_world.SecretTypeID:   s4wave_secret_world.SecretType,
	bldr_manifest_world.ManifestTypeID: objecttype.NewObjectType(bldr_manifest_world.ManifestTypeID, s4wave_forge_world.ForgeReadOnlyFactory),
})

func spaceSettingsReadOnlyFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	return nil, func() {}, nil
}
