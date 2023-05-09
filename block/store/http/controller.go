package block_store_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/controller"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_controller "github.com/aperturerobotics/hydra/block/store/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the http block store controller.
const ControllerID = "hydra/block/store/http"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the http block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new http block store controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "http block store"),
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
	return func(ctx context.Context, released func()) (*block_store.Store, func(), error) {
		baseURL, err := conf.ParseURL()
		if err != nil {
			return nil, nil, err
		}
		httpBlock := NewHTTPBlock(ctx, !conf.GetReadOnly(), http.DefaultClient, baseURL, conf.GetForceHashType())
		var store block_store.Store = httpBlock
		return &store, nil, nil
	}
}
