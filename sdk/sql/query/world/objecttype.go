//go:build !tinygo

package s4wave_sql_query_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlQueryType registers the SQL query world ObjectType.
var SqlQueryType = objecttype.NewObjectType(s4wave_sql_query.SqlQueryTypeID, SqlQueryFactory)

// SqlQueryFactory opens a SQL query object over SRPC.
func SqlQueryFactory(
	ctx context.Context,
	_ *logrus.Entry,
	_ bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_sql_query.SqlQueryTypeID); err != nil {
		return nil, nil, err
	}

	resource := NewSqlQueryResource(ws, engine, objectKey)
	return resource.GetMux(), resource.Close, nil
}
