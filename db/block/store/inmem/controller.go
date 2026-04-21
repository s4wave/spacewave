package block_store_inmem

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the inmem block store controller.
const ControllerID = "hydra/block/store/inmem"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the inmem block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new inmem block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "inmem block store"),
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
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		kvk, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
		if err != nil {
			return nil, nil, err
		}
		st := store_kvtx_inmem.NewStore()
		storeOps := NewInmemBlock(kvk, st, conf.GetForceHashType(), conf.GetHashGet())
		store := block_store.NewStore(conf.GetBlockStoreId(), storeOps)
		return store, nil, nil
	}
}
