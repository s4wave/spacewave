package block_store_ristretto

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	store_kvtx_ristretto "github.com/aperturerobotics/hydra/store/kvtx/ristretto"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the ristretto block store controller.
const ControllerID = "hydra/block/store/ristretto"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the ristretto block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new ristretto block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "ristretto block cache"),
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
		st, err := store_kvtx_ristretto.NewStore(conf.GetRistretto())
		if err != nil {
			return nil, nil, err
		}
		kvtxBlk := NewRistrettoBlock(kvk, st, conf.GetForceHashType(), conf.GetHashGet())
		var store block_store.Store = block_store.NewStore(conf.GetBlockStoreId(), kvtxBlk)
		return store, st.Close, nil
	}
}
