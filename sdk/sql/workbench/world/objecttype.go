//go:build !tinygo

package s4wave_sql_workbench_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlWorkbenchType registers the SQL workbench world ObjectType.
var SqlWorkbenchType = objecttype.NewObjectType(
	s4wave_sql_workbench.SqlWorkbenchTypeID,
	SqlWorkbenchFactory,
)

// SqlWorkbenchFactory opens a SQL workbench object over SRPC.
func SqlWorkbenchFactory(
	ctx context.Context,
	_ *logrus.Entry,
	_ bus.Bus,
	_ world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_sql_workbench.SqlWorkbenchTypeID); err != nil {
		return nil, nil, err
	}

	resource := NewSqlWorkbenchResource(ws, objectKey)
	return resource.GetMux(), resource.Close, nil
}
