package s4wave_canvas_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// CanvasTypeID is the object type ID for canvas objects.
const CanvasTypeID = "canvas"

// CanvasType is the ObjectType for canvas objects.
var CanvasType = objecttype.NewObjectType(CanvasTypeID, CanvasFactory)

// CanvasFactory creates a CanvasResource from a world object.
func CanvasFactory(
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

	// Read the current canvas state from the world object.
	var state *s4wave_canvas.CanvasState
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		state, err = s4wave_canvas.UnmarshalCanvasState(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	if state == nil {
		state = &s4wave_canvas.CanvasState{}
	}

	resource := s4wave_canvas.NewCanvasResource(ws, engine, objectKey, state)
	return resource.GetMux(), func() {}, nil
}
