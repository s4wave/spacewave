package s4wave_kv_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	kvtx_rpc "github.com/s4wave/spacewave/db/kvtx/rpc"
	kvtx_rpc_server "github.com/s4wave/spacewave/db/kvtx/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// KvStoreTypeID is the world ObjectType identifier for the KVTX store surface.
const KvStoreTypeID = "kv/store"

// KvStoreType is the registered object type for KVTX stores.
var KvStoreType = objecttype.NewObjectType(KvStoreTypeID, KvStoreFactory)

// KvStoreFactory opens a KVTX RPC service for a kv/store world object.
func KvStoreFactory(
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
	if err := world_types.CheckObjectType(ctx, ws, objectKey, KvStoreTypeID); err != nil {
		return nil, nil, err
	}

	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	var store *WorldBackedStore
	owned, err := obj.BuildOwnedLookupCursor(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	store, err = NewWorldBackedStore(ctx, le, owned, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, errors.New("kv/store: failed to open world-backed store")
	}

	mux := srpc.NewMux()
	if err := kvtx_rpc.SRPCRegisterKvtx(mux, kvtx_rpc_server.NewStore(store)); err != nil {
		store.Close()
		return nil, nil, err
	}
	return mux, store.Close, nil
}
