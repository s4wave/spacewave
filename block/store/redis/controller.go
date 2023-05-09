package block_store_redis

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the redis block store controller.
const ControllerID = "hydra/block/store/redis"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the redis block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new redis block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "redis block store"),
		NewBlockStoreBuilder(le, conf),
		[]string{conf.GetBlockStoreId()},
		true,
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
func NewBlockStoreBuilder(le *logrus.Entry, conf *Config) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (*block_store.Store, func(), error) {
		kvk, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
		if err != nil {
			return nil, nil, err
		}
		st, err := conf.GetClient().Connect(ctx)
		if err != nil {
			return nil, nil, err
		}
		kvtxBlk := NewRedisBlock(ctx, kvk, st, conf.GetForceHashType())
		var store block_store.Store = kvtxBlk
		return &store, func() { st.GetPool().Close() }, nil
	}
}
