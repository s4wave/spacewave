package provider_local

import (
	"sync"

	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
)

// fillCaches maps the storage volume ID of an account to the fill cache its
// placed block stores read through, as SetFillCache registered it.
var fillCaches sync.Map

// SetFillCache gives the placed block stores of the account
// providerID/accountID a fill cache: a block read from the backend's packs is
// copied into cache instead of the account's volume, and later reads find it
// there before the packs. It must be called before the account starts, since
// the account mounts its placed block stores as it starts. A nil cache returns
// the account to filling its volume. The caller keeps cache open while the
// account runs.
func SetFillCache(providerID, accountID string, cache *block_store_fillcache.Cache) {
	id := StorageVolumeID(providerID, accountID)
	if cache == nil {
		fillCaches.Delete(id)
		return
	}
	fillCaches.Store(id, cache)
}

// fillCache returns the fill cache registered for the block store's account,
// or nil.
func (t *bstoreTracker) fillCache() *block_store_fillcache.Cache {
	id := StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID())
	if cache, ok := fillCaches.Load(id); ok {
		return cache.(*block_store_fillcache.Cache)
	}
	return nil
}
