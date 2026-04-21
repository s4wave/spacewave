package block_store_rpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_server "github.com/s4wave/spacewave/db/block/rpc/server"
	block_store "github.com/s4wave/spacewave/db/block/store"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ControllerID is the controller identifier.
const ControllerID = "hydra/block/store/rpc/server"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description
var controllerDescrip = "serves block store via rpc"

// Controller is the block store rpc service controller.
//
// Handles LookupRpcService with the block store endpoints.
type Controller = bifrost_rpc.RpcServiceController

// NewController constructs a new rpc handler controller.
func NewController(b bus.Bus, conf *Config) *Controller {
	return bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(
			ControllerID,
			Version,
			controllerDescrip,
		),
		NewRpcServiceBuilder(b, conf),
		nil,
		false,
		nil,
		[]string{conf.GetServiceId()},
		nil,
	)
}

// NewRpcServiceBuilder constructs a new rpc service builder from config.
func NewRpcServiceBuilder(b bus.Bus, conf *Config) bifrost_rpc.RpcServiceBuilder {
	return func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
		// Lookup the block store.
		bsv, _, ref, err := block_store.ExLookupFirstBlockStore(ctx, b, conf.GetBlockStoreId(), false, released)
		if err != nil {
			return nil, nil, err
		}

		mux := srpc.NewMux()
		if err := mux.Register(
			block_rpc.NewSRPCBlockStoreHandler(
				block_rpc_server.NewBlockStore(bsv),
				conf.GetServiceId(),
			),
		); err != nil {
			ref.Release()
			return nil, nil, err
		}

		var handler srpc.Invoker = mux
		return handler, ref.Release, nil
	}
}
