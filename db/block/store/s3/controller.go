//go:build !tinygo

package block_store_s3

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the s3 block store controller.
const ControllerID = "hydra/block/store/s3"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the s3 block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new s3 block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "s3 block store"),
		NewBlockStoreBuilder(conf),
		[]string{conf.GetBlockStoreId()},
		true,
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
func NewBlockStoreBuilder(conf *Config) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		client, err := BuildClient(conf.GetClient())
		if err != nil {
			return nil, nil, err
		}
		s3Block := NewS3Block(
			!conf.GetReadOnly(),
			client,
			conf.GetBucketName(),
			conf.GetObjectPrefix(),
			conf.GetForceHashType(),
		)
		store := block_store.NewStore(conf.GetBlockStoreId(), s3Block)
		return store, nil, nil
	}
}
