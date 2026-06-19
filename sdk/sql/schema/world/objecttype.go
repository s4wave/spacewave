//go:build !tinygo && !sql_lite

package s4wave_sql_schema_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlSchemaType registers the SQL schema world ObjectType.
var SqlSchemaType = objecttype.NewObjectType(s4wave_sql_schema.SqlSchemaTypeID, SqlSchemaFactory)

// SqlSchemaFactory opens a SQL schema object over SRPC.
func SqlSchemaFactory(
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
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_sql_schema.SqlSchemaTypeID); err != nil {
		return nil, nil, err
	}

	resource := NewSqlSchemaResource(ws, objectKey)
	return resource.GetMux(), resource.Close, nil
}
