package s4wave_drive_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_drive "github.com/s4wave/spacewave/sdk/space/drive"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// DriveTypeID is the object type ID for Drive objects.
const DriveTypeID = s4wave_drive.DriveTypeID

// DriveType is the ObjectType for Drive objects.
var DriveType = objecttype.NewObjectType(DriveTypeID, driveFactory)

// driveFactory creates a DriveResource from a world object.
func driveFactory(
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

	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		_, err := s4wave_drive.UnmarshalDrive(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_drive.NewDriveResource(ws, objectKey)
	return resource.GetMux(), func() {}, nil
}
