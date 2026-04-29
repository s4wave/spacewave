package cdn_bstore_controller

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the anonymous CDN block store controller.
const ControllerID = "spacewave/cdn/bstore"

// Version is the version of the block store implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the anonymous CDN block store controller.
type Controller = block_store_controller.Controller

// NewController builds a new anonymous CDN block store controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "anonymous CDN block store"),
		NewBlockStoreBuilder(le, b, conf),
		[]string{conf.GetBlockStoreId()},
		true,
		conf.GetBucketIds(),
		conf.GetSkipNotFound(),
		conf.GetVerbose(),
	)
}

// NewBlockStoreBuilder constructs a new block store builder from config.
func NewBlockStoreBuilder(le *logrus.Entry, b bus.Bus, conf *Config) block_store_controller.BlockStoreBuilder {
	return func(ctx context.Context, released func()) (block_store.Store, func(), error) {
		pointerTTL, _ := conf.ParsePointerTTLDur()
		cdnStore, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
			CdnBaseURL: conf.GetCdnBaseUrl(),
			SpaceID:    conf.GetSpaceId(),
			HttpClient: http.DefaultClient,
			PointerTTL: pointerTTL,
		})
		if err != nil {
			return nil, nil, err
		}
		if maxBytes := conf.GetRangeCacheMaxBytes(); maxBytes > 0 {
			cdnStore.SetRangeCacheMaxBytes(maxBytes)
		}

		var rel func()
		if cacheID := conf.GetCacheBlockStoreId(); cacheID != "" {
			cacheStore, _, cacheRef, err := block_store.ExLookupFirstBlockStore(ctx, b, cacheID, false, released)
			if err != nil {
				return nil, nil, err
			}
			cdnStore.SetWriteback(ctx, cacheStore, conf.GetWritebackWindowBytes())
			rel = cacheRef.Release
		}

		store := block_store.NewStore(conf.GetBlockStoreId(), cdnStore)
		return store, rel, nil
	}
}
