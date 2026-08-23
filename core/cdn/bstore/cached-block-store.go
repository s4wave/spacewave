package cdn_bstore

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/volume"
)

// CachedBlockStoreOptions configure NewCachedBlockStore.
type CachedBlockStoreOptions struct {
	// CdnBaseURL is the public CDN origin (e.g. https://cdn.spacewave.app).
	CdnBaseURL string
	// SpaceID is the CDN Space ULID.
	SpaceID string
	// CacheBlockStoreID is the bus block store used for writeback and
	// pack-index persistence. Empty disables the local cache.
	CacheBlockStoreID string
	// PointerTTL is the cache TTL for the decoded root pointer. See Options.
	PointerTTL time.Duration
	// WritebackWindowBytes bounds co-block writeback buffering.
	WritebackWindowBytes int64
	// HttpClient overrides the default http.Client.
	HttpClient *http.Client
	// Release is called when a looked-up cache dependency is disposed while
	// the assembled store is still live. Callers whose store sits under a
	// lifecycle owner pass their invalidation callback here. Callers that
	// own the store for their whole lifetime may leave it nil; the bus
	// references are still freed by the returned release function.
	Release func()
}

// NewCachedBlockStore assembles a CdnBlockStore with its local cache wiring.
//
// With CacheBlockStoreID set it looks up the writeback block store and the
// durable pack-index object store on the bus, wires both into the CDN store,
// and returns a release function that closes the store and frees the bus
// references. On assembly failure every acquired reference is freed before
// the error returns.
func NewCachedBlockStore(ctx context.Context, b bus.Bus, opts CachedBlockStoreOptions) (*CdnBlockStore, func(), error) {
	var indexCache packfile_store.IndexCache
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
	if cacheID := opts.CacheBlockStoreID; cacheID != "" {
		var err error
		var cacheRef directive.Reference
		cacheStore, _, cacheRef, err = block_store.ExLookupFirstBlockStore(ctx, b, cacheID, false, opts.Release)
		if err != nil {
			return nil, nil, err
		}
		releaseCache = cacheRef.Release
		objHandle, _, objRef, err := volume.ExBuildObjectStoreAPI(
			ctx,
			b,
			true,
			PackIndexObjectStoreID(opts.SpaceID),
			cacheID,
			opts.Release,
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
	store, err := NewCdnBlockStore(Options{
		CdnBaseURL: opts.CdnBaseURL,
		SpaceID:    opts.SpaceID,
		HttpClient: opts.HttpClient,
		PointerTTL: opts.PointerTTL,
		IndexCache: indexCache,
	})
	if err != nil {
		releaseRefs()
		return nil, nil, err
	}
	if cacheStore != nil {
		store.SetWriteback(ctx, cacheStore, opts.WritebackWindowBytes)
	}
	return store, func() {
		store.Close()
		releaseRefs()
	}, nil
}
