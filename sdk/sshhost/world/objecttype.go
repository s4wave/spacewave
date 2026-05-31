package s4wave_sshhost_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SshHostTypeID is the type identifier for SSH-only Host objects.
const SshHostTypeID = s4wave_sshhost.SshHostTypeID

// SshHostType is the ObjectType for SSH-only Host objects.
var SshHostType = objecttype.NewObjectType(SshHostTypeID, sshHostFactory)

func sshHostFactory(
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

	var state *s4wave_sshhost.SshHost
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = s4wave_sshhost.UnmarshalSshHost(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_sshhost.NewSshHostResource(ws, objectKey, state)
	return resource.GetMux(), func() {}, nil
}
