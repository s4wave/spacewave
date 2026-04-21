package block_store_overlay

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the overlay block store controller.
const ControllerID = "hydra/block/store/overlay"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the overlay block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new overlay block store controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "overlay block store"),
		NewBlockStoreBuilder(le, b, conf),
		[]string{conf.GetBlockStoreId()},
		true,
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
//
// This builder is designed to only return a value once lower and upper are resolved.
func NewBlockStoreBuilder(le *logrus.Entry, b bus.Bus, conf *Config) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		// Parse the writeback timeout
		// We assert this does not return an error in the config.Validate function.
		writebackTimeout, _ := conf.ParseWritebackTimeoutDur()

		// Lookup the lower block store.
		lowerBlockStore, _, lowerBlockStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, b, conf.GetLowerBlockStoreId(), false, released)
		if err != nil {
			return nil, nil, err
		}

		// Lookup the upper block store.
		upperBlockStore, _, upperBlockStoreRef, err := block_store.ExLookupFirstBlockStore(ctx, b, conf.GetUpperBlockStoreId(), false, released)
		if err != nil {
			lowerBlockStoreRef.Release()
			return nil, nil, err
		}

		// Construct the overlay
		overlayBlock := NewOverlayBlock(ctx, lowerBlockStore, upperBlockStore, conf.GetOverlayMode(), writebackTimeout, conf.GetWritebackPutOpts())
		store := block_store.NewStore(conf.GetBlockStoreId(), overlayBlock)
		return store, func() {
			upperBlockStoreRef.Release()
			lowerBlockStoreRef.Release()
		}, nil
	}
}
