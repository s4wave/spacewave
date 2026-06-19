//go:build !tinygo && !sql_lite

package s4wave_sql_table_view_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlTableViewType registers the SQL table view world ObjectType.
var SqlTableViewType = objecttype.NewObjectType(
	s4wave_sql_table_view.SqlTableViewTypeID,
	SqlTableViewFactory,
)

// SqlTableViewFactory opens a SQL table view object over SRPC.
func SqlTableViewFactory(
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
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_sql_table_view.SqlTableViewTypeID); err != nil {
		return nil, nil, err
	}

	resource := NewSqlTableViewResource(ws, objectKey)
	return resource.GetMux(), resource.Close, nil
}
