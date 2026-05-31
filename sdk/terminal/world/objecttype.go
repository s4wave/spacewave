package s4wave_terminal_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// TerminalTypeID is the type identifier for remote Terminal objects.
const TerminalTypeID = s4wave_terminal.TerminalTypeID

// TerminalType is the ObjectType for remote Terminal objects.
var TerminalType = objecttype.NewObjectType(TerminalTypeID, terminalFactory)

func terminalFactory(
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

	var state *s4wave_terminal.Terminal
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_terminal.UnmarshalTerminal(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_terminal.NewTerminalResource(b, ws, engine, objectKey, state)
	return resource.GetMux(), func() {}, nil
}
