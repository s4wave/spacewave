package s4wave_secret_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SecretTypeID is the ObjectType id for Secret objects.
const SecretTypeID = s4wave_secret.SecretTypeID

// SecretType is the ObjectType for Secret objects.
var SecretType = objecttype.NewObjectType(SecretTypeID, SecretFactory)

// SecretFactory creates a SecretResource from a world object.
func SecretFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	_ world.Engine,
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
		_, err := s4wave_secret.UnmarshalSecret(ctx, bcs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	resource := s4wave_secret.NewSecretResource(le, b, ws, objectKey)
	return resource.GetMux(), func() {}, nil
}
