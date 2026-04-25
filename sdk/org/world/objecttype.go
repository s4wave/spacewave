package s4wave_org_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// OrganizationTypeID is the type identifier for organization objects.
const OrganizationTypeID = s4wave_org.OrganizationTypeID

// OrganizationType is the ObjectType for organization objects.
var OrganizationType = objecttype.NewObjectType(OrganizationTypeID, orgFactory)

// orgFactory creates an OrgResource from a world object.
func orgFactory(
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

	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	var state *s4wave_org.OrgState
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		state, err = s4wave_org.UnmarshalOrgState(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_org.NewOrgResource(ws, objectKey, state)
	return resource.GetMux(), func() {}, nil
}
