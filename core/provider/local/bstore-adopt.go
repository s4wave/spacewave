package provider_local

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// ErrAdoptServeOnly is returned by AdoptBucketPack for a store in serve mode,
// whose index another process writes.
var ErrAdoptServeOnly = errors.New("store is in serve mode: it adopts no packs")

// bucketAdopter is the pack store's adoption of a pack already in its bucket.
type bucketAdopter interface {
	// AdoptBucketPack verifies and registers the pack entry names.
	AdoptBucketPack(ctx context.Context, entry *packfile.PackfileEntry) (bool, error)
	// HasPackfile reports whether the store holds pack id registered.
	HasPackfile(ctx context.Context, id string) (bool, error)
}

// adoptKey names one mounted block store of an account.
type adoptKey struct {
	volumeID string
	storeID  string
}

// bucketAdopters maps each mounted block store to its storeAdopter.
var bucketAdopters sync.Map

// storeAdopter adopts packs into one mounted store's open backend.
type storeAdopter struct {
	t *bstoreTracker
	p *placementState
}

// registerAdopter makes the tracker's store reachable by AdoptBucketPack and
// returns the function that removes it.
func (t *bstoreTracker) registerAdopter(p *placementState) func() {
	key := adoptKey{volumeID: StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()), storeID: t.id}
	value := &storeAdopter{t: t, p: p}
	bucketAdopters.Store(key, value)
	return func() { bucketAdopters.CompareAndDelete(key, value) }
}

// backend returns the store's open backend as a bucket adopter. The backend
// is resolved per call, so a backend swapped since the last call is used.
func (a *storeAdopter) backend() (bucketAdopter, error) {
	remote, _ := a.t.readBackend(a.p)
	if remote == nil {
		return nil, errors.New("storage backend not open")
	}
	adopter, ok := remote.(bucketAdopter)
	if !ok {
		return nil, errors.New("storage backend adopts no packs")
	}
	return adopter, nil
}

// lookupAdopter returns the mounted store's adopter.
func lookupAdopter(providerID, accountID, storeID string) (*storeAdopter, error) {
	value, ok := bucketAdopters.Load(adoptKey{volumeID: StorageVolumeID(providerID, accountID), storeID: storeID})
	if !ok {
		return nil, errors.Errorf("block store %q is not mounted", storeID)
	}
	return value.(*storeAdopter), nil
}

// AdoptBucketPack registers a pack another writer uploaded to the bucket of
// the mounted block store storeID of the account, in the running store's own
// index, so its blocks read without a restart. It is the pack store's
// AdoptBucketPack, run in this process: the index has one writer, and the
// in-memory pack table that numbers packs stays the only one.
//
// The located-block clear the node adopter runs is skipped: a pack adopted
// from the bucket was not written by this store's upload queue.
func AdoptBucketPack(ctx context.Context, providerID, accountID, storeID string, entry *packfile.PackfileEntry) (registered bool, err error) {
	a, err := lookupAdopter(providerID, accountID, storeID)
	if err != nil {
		return false, err
	}
	if a.p.serveOnly {
		return false, ErrAdoptServeOnly
	}
	backend, err := a.backend()
	if err != nil {
		return false, err
	}
	return backend.AdoptBucketPack(ctx, entry)
}

// HasPackfile reports whether the mounted block store storeID of the account
// holds pack id registered in its open backend.
func HasPackfile(ctx context.Context, providerID, accountID, storeID, id string) (bool, error) {
	a, err := lookupAdopter(providerID, accountID, storeID)
	if err != nil {
		return false, err
	}
	backend, err := a.backend()
	if err != nil {
		return false, err
	}
	return backend.HasPackfile(ctx, id)
}
