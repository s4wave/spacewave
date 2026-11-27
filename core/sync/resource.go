package sync

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	"github.com/sirupsen/logrus"
)

// AttachEngine mounts a granted engine as a Resource service for a private host.
// Its caller owns the RPC context. Cancel and join that context before release.
// Releasing the mount leaves the caller's engine usable.
func AttachEngine(le *logrus.Entry, b bus.Bus, engine world.Engine) (srpc.Invoker, func(), error) {
	root := resource_world.NewMount(le, b, lookupOp).NewEngineResource(engine, &sdk_world.EngineInfo{})
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(root.GetMux(), "").Register(mux); err != nil {
		root.Close()
		return nil, nil, err
	}
	return mux, root.Close, nil
}

// registerKV makes the ordinary KV ObjectType available to this engine.
func registerKV(ctx context.Context, b bus.Bus) error {
	_, err := b.AddController(ctx, objecttype_controller.NewController(func(_ context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == kv_world.KvStoreTypeID {
			return kv_world.KvStoreType, nil
		}
		return nil, nil
	}), nil)
	return err
}

// lookupOp resolves the operation carried by the standard KV Resource.
func lookupOp(_ context.Context, id string) (world.Operation, error) {
	if id == kv_world.KvSetRootOpTypeID {
		return &kv_world.KvSetRootOp{}, nil
	}
	return nil, nil
}
