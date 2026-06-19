//go:build !tinygo && !sql_lite

package s4wave_sql_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_server "github.com/s4wave/spacewave/db/sql/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// SqlDbTypeID is the world ObjectType id for SQL databases.
const SqlDbTypeID = "sql/db"

// SqlDbType registers the SQL database world ObjectType.
var SqlDbType = objecttype.NewObjectType(SqlDbTypeID, SqlDbFactory)

// SqlDbFactory opens a world-backed SQL database object over SRPC.
func SqlDbFactory(
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
	if err := world_types.CheckObjectType(ctx, ws, objectKey, SqlDbTypeID); err != nil {
		return nil, nil, err
	}
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	var store *WorldBackedSql
	if err := obj.AccessWorldState(ctx, nil, func(root worldCursor) error {
		store, err = NewWorldBackedSql(ctx, root.Clone(), ws, objectKey)
		return err
	}); err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, errors.New("sql/db: failed to open world-backed database")
	}

	mux := srpc.NewMux()
	if err := sql_rpc.SRPCRegisterSql(mux, sql_rpc_server.NewStore(store)); err != nil {
		store.Close()
		return nil, nil, err
	}
	return mux, store.Close, nil
}
