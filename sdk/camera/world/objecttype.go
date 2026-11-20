package s4wave_camera_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_camera "github.com/s4wave/spacewave/sdk/camera"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// CameraTypeID is the type identifier for Camera objects.
const CameraTypeID = s4wave_camera.CameraTypeID

// CameraType is the ObjectType for Camera objects.
var CameraType = objecttype.NewObjectType(CameraTypeID, cameraFactory)

// cameraFactory decodes the Camera at objectKey and serves its Resource to
// the accessing Session.
func cameraFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Require World state before resolving the Camera object.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Acquire the Camera object state.
	objState, found, err := ws.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	// Decode the Camera record within the object read scope.
	var camera *s4wave_camera.Camera
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		camera, uerr = s4wave_camera.UnmarshalCamera(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, nil, err
	}

	// Expose the Camera Resource through its RPC mux.
	resource := s4wave_camera.NewCameraResource(b, ws, objecttype.SessionPeerIDFromContext(ctx), camera)
	return resource.GetMux(), func() {}, nil
}
