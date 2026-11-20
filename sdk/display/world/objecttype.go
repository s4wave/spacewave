package s4wave_display_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_display "github.com/s4wave/spacewave/sdk/display"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// DisplayTypeID is the type identifier for Display objects.
const DisplayTypeID = s4wave_display.DisplayTypeID

// DisplayType is the ObjectType for Display objects.
var DisplayType = objecttype.NewObjectType(DisplayTypeID, displayFactory)

// displayFactory reads the Display and opens its resource for the viewer.
func displayFactory(
	ctx context.Context,
	_ *logrus.Entry,
	b bus.Bus,
	_ world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	// Decode the Display within the accessing Session's World state.
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	display, err := world.LookupObjectBody[s4wave_display.Display](ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	// Expose the Display resource through its RPC mux.
	resource := s4wave_display.NewDisplayResource(b, ws, objecttype.SessionPeerIDFromContext(ctx), display)
	return resource.GetMux(), func() {}, nil
}
