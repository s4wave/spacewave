package provider_local

import (
	"sync"

	"github.com/s4wave/spacewave/db/block"
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

// packFronts maps the storage volume ID of an account to the read-only store
// its placed block stores read before the backend's packs, as SetPackFront
// registered it.
var packFronts sync.Map

// SetPackFront gives the placed block stores of the account
// providerID/accountID a read-only store to read a block from after the
// account's own store misses and before the fill cache and the backend's
// packs, such as an earlier local copy of the account's blocks. Its hits fill
// nothing, since the store is local already. It must be called before the
// account starts. A nil store removes it; the caller keeps it open while the
// account runs.
func SetPackFront(providerID, accountID string, front block.StoreOps) {
	id := StorageVolumeID(providerID, accountID)
	if front == nil {
		packFronts.Delete(id)
		return
	}
	packFronts.Store(id, front)
}

// packFront returns the read-only store registered before the packs of the
// block store's account, or nil.
func (t *bstoreTracker) packFront() block.StoreOps {
	id := StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID())
	if front, ok := packFronts.Load(id); ok {
		return front.(block.StoreOps)
	}
	return nil
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
