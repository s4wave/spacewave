package cdn_bstore_controller

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the anonymous CDN block store controller.
const ControllerID = "spacewave/cdn/bstore"

// Version is the version of the block store implementation.
var Version = controller.MustParseVersion("0.0.1")

// Controller implements the anonymous CDN block store controller.
type Controller = block_store_controller.Controller

type blockStoreHandle struct {
	block_store.Store
	cdnStore *cdn_bstore.CdnBlockStore
}

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
		var cacheStore block_store.LookupBlockStoreValue
		var releaseCache, releaseIndex func()
		releaseRefs := func() {
			if releaseIndex != nil {
				releaseIndex()
			}
			if releaseCache != nil {
				releaseCache()
			}
		}
		var indexCache packfile_store.IndexCache
		if cacheID := conf.GetCacheBlockStoreId(); cacheID != "" {
			var err error
			var cacheRef directive.Reference
			cacheStore, _, cacheRef, err = block_store.ExLookupFirstBlockStore(ctx, b, cacheID, false, released)
			if err != nil {
				return nil, nil, err
			}
			releaseCache = cacheRef.Release
			objHandle, _, objRef, err := volume.ExBuildObjectStoreAPI(
				ctx,
				b,
				true,
				cdn_bstore.PackIndexObjectStoreID(conf.GetSpaceId()),
				cacheID,
				released,
			)
			if err != nil {
				releaseRefs()
				return nil, nil, err
			}
			if objHandle != nil {
				releaseIndex = objRef.Release
				indexCache = manifest.NewIndexCache(objHandle.GetObjectStore())
			}
		}
		cdnStore, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
			CdnBaseURL: conf.GetCdnBaseUrl(),
			SpaceID:    conf.GetSpaceId(),
			HttpClient: http.DefaultClient,
			PointerTTL: pointerTTL,
			IndexCache: indexCache,
		})
		if err != nil {
			releaseRefs()
			return nil, nil, err
		}
		if maxBytes := conf.GetRangeCacheMaxBytes(); maxBytes > 0 {
			cdnStore.SetRangeCacheMaxBytes(maxBytes)
		}
		if cacheStore != nil {
			cdnStore.SetWriteback(ctx, cacheStore, conf.GetWritebackWindowBytes())
		}

		store := &blockStoreHandle{
			Store:    block_store.NewStore(conf.GetBlockStoreId(), cdnStore),
			cdnStore: cdnStore,
		}
		return store, func() {
			cdnStore.Close()
			releaseRefs()
		}, nil
	}
}
