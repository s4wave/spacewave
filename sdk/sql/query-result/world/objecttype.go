//go:build !tinygo && !sql_lite

package s4wave_sql_query_result_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlQueryResultType registers the SQL query result world ObjectType.
var SqlQueryResultType = objecttype.NewObjectType(
	s4wave_sql_query_result.SqlQueryResultTypeID,
	SqlQueryResultFactory,
)

// SqlQueryResultFactory opens a SQL query result object over SRPC.
func SqlQueryResultFactory(
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
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_sql_query_result.SqlQueryResultTypeID); err != nil {
		return nil, nil, err
	}

	resource := s4wave_sql_query_result.NewSqlQueryResultResource(ws, objectKey)
	return resource.GetMux(), resource.Close, nil
}
