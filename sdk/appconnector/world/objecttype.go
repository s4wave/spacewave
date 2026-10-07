// Package s4wave_appconnector_world registers the AppConnector and AppSnapshot ObjectTypes.
package s4wave_appconnector_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	s4wave_appconnector "github.com/s4wave/spacewave/sdk/appconnector"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// AppConnectorTypeID is the type identifier for AppConnector objects.
const AppConnectorTypeID = s4wave_appconnector.AppConnectorTypeID

// AppSnapshotTypeID is the type identifier for AppSnapshot objects.
const AppSnapshotTypeID = s4wave_appconnector.AppSnapshotTypeID

// AppConnectorType is the ObjectType for AppConnector objects.
// Its factory serves a PersistentExecutionService that keeps the connector's
// AppSnapshot current.
var AppConnectorType = objecttype.NewObjectType(AppConnectorTypeID, connectorFactory)

// AppSnapshotType is the ObjectType for AppSnapshot objects.
var AppSnapshotType = objecttype.NewObjectType(AppSnapshotTypeID, snapshotFactory)

// snapshotFactory serves no resource: viewers read the snapshot block directly.
func snapshotFactory(
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
