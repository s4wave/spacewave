//go:build !tinygo && !goscript

package objecttypes

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/provider/spacewave/cloudinbox"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_org_world "github.com/s4wave/spacewave/sdk/org/world"
	s4wave_secret_world "github.com/s4wave/spacewave/sdk/secret/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

var compiledObjectTypes = extendObjectTypes(commonObjectTypes, map[string]objecttype.ObjectType{
	space_world.SpaceSettingsTypeID:  objecttype.NewObjectType(space_world.SpaceSettingsTypeID, spaceSettingsReadOnlyFactory),
	s4wave_org.OrgStateTypeID:        s4wave_org_world.OrganizationType,
	s4wave_secret_world.SecretTypeID: s4wave_secret_world.SecretType,
	cloudinbox.TypeID:                provider_spacewave.CloudInboxType,
})

// compiledObjectTypeMetadata is the metadata of the compiled types a Flowgraph
// binds by their ports.
var compiledObjectTypeMetadata = map[string]*s4wave_objecttype_registry.ObjectTypeMetadata{
	cloudinbox.TypeID: cloudinbox.Metadata(),
}

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
