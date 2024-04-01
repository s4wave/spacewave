package block_store_rpc

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	block_rpc "github.com/aperturerobotics/hydra/block/rpc"
	block_rpc_client "github.com/aperturerobotics/hydra/block/rpc/client"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the rpc block store controller.
const ControllerID = "hydra/block/store/rpc"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the rpc block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new rpc block store controller.
func NewController(b bus.Bus, le *logrus.Entry, conf *Config) *Controller {
	var matchBlockStoreIDs []string
	if id := conf.GetBlockStoreId(); id != "" {
		matchBlockStoreIDs = append(matchBlockStoreIDs, id)
	}
	matchBlockStoreIDs = append(matchBlockStoreIDs, conf.GetBlockStoreIds()...)
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "rpc block store"),
		NewBlockStoreBuilder(b, conf),
		matchBlockStoreIDs,
		conf.GetLookupOnStart(),
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
func NewBlockStoreBuilder(b bus.Bus, conf *Config) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		serviceID, clientID := conf.GetServiceId(), conf.GetClientId()
		clientSet, _, clientSetRef, err := bifrost_rpc.ExLookupRpcClientSet(ctx, b, serviceID, clientID, true, released)
		if err != nil {
			return nil, nil, err
		}
		blockClient := block_rpc.NewSRPCBlockStoreClientWithServiceID(clientSet, serviceID)
		blockStore := block_rpc_client.NewBlockStore(blockClient, conf.GetForceHashType(), conf.GetReadOnly())
		return block_store.NewStore(conf.GetBlockStoreId(), blockStore), clientSetRef.Release, nil
	}
}
