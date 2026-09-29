package s4wave_volume_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/tx"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// VolumeType exposes the ordinary Volume, BlockStore, and ObjectStore services.
var VolumeType = objecttype.NewObjectType(volume_world.ObjectTypeID, VolumeFactory)

// VolumeFactory opens the explicitly linked KV backing under the granted World.
// Access to this Resource includes the installation identity stored in the Volume.
func VolumeFactory(ctx context.Context, le *logrus.Entry, b bus.Bus, engine world.Engine,
	ws world.WorldState, key string,
) (srpc.Invoker, func(), error) {
	// Require a writable World state.
	if ws == nil || engine == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	if ws.GetReadOnly() {
		return nil, nil, tx.ErrNotWrite
	}

	// Load the granted backing and open its Volume on the engine.
	backing, err := volume_world.LoadBacking(ctx, ws, key)
	if err != nil {
		return nil, nil, err
	}
	vol, err := volume_world.NewVolumeWithEngine(ctx, le, b, transform_all.BuildFactorySet(),
		&volume_world.Config{ObjectKey: backing.KvObjectKey}, engine)
	if err != nil {
		return nil, nil, err
	}

	// Register the Volume's proxy service on a new RPC mux.
	mux := srpc.NewMux()
	proxy := volume_rpc_server.NewProxyVolume(ctx, vol, true)
	if err := volume_rpc_server.RegisterProxyVolume(mux, proxy); err != nil {
		_ = vol.Close()
		return nil, nil, err
	}
	return mux, func() { _ = vol.Close() }, nil
}
