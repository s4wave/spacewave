//go:build !tinygo

package s4wave_sql_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	sql_rpc_server "github.com/s4wave/spacewave/db/sql/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

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
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, nil, err
	}
	storageRoot, err := ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, nil, err
	}
	root, err := storageRoot.FollowRef(ctx, rootRef)
	if err != nil {
		storageRoot.Release()
		return nil, nil, err
	}

	store, err := NewWorldBackedSql(ctx, root, ws, objectKey)
	if err != nil {
		root.Release()
		storageRoot.Release()
		return nil, nil, err
	}

	mux := srpc.NewMux()
	if err := sql_rpc.SRPCRegisterSql(mux, sql_rpc_server.NewStore(store)); err != nil {
		store.Close()
		storageRoot.Release()
		return nil, nil, err
	}
	return mux, func() {
		store.Close()
		storageRoot.Release()
	}, nil
}
