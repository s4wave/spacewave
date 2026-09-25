package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
)

// settingsWatch follows the account settings.
type settingsWatch struct {
	// states is the account settings state.
	states ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
	// snapshot is the last settings snapshot read.
	snapshot sobject.SharedObjectStateSnapshot
	// release releases the settings mount.
	release func()
}

// lookupAccountSettingsRef returns the account settings SharedObject, or nil
// when the account has no settings yet.
func (a *ProviderAccount) lookupAccountSettingsRef(ctx context.Context) (*sobject.SharedObjectRef, error) {
	ref, err := a.GetAccountSettingsRef(ctx)
	if errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return nil, nil
	}
	return ref, err
}

// watchAccountSettings mounts the account settings for watching.
func (a *ProviderAccount) watchAccountSettings(ctx context.Context, ref *sobject.SharedObjectRef) (*settingsWatch, error) {
	so, releaseSo, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		releaseSo()
		return nil, err
	}
	return &settingsWatch{
		states: states,
		release: func() {
			releaseStates()
			releaseSo()
		},
	}, nil
}

// next waits for the next settings change and returns the settings.
func (w *settingsWatch) next(ctx context.Context) (*account_settings.AccountSettings, error) {
	snapshot, err := w.states.WaitValueChange(ctx, w.snapshot, nil)
	if err != nil {
		return nil, err
	}
	w.snapshot = snapshot
	settings, _, err := decodeAccountSettingsSnapshot(ctx, snapshot)
	return settings, err
}

// nextBackend waits for the next settings change and returns the storage
// backend holding the store, or nil when the store is on the account's own
// storage.
func (t *bstoreTracker) nextBackend(ctx context.Context, watch *settingsWatch) (*account_settings.StorageBackend, error) {
	settings, err := watch.next(ctx)
	if err != nil {
		return nil, err
	}
	placement := settings.FindBlockStorePlacement(t.id)
	if placement == nil {
		return nil, nil
	}
	backend := settings.FindStorageBackend(placement.GetStorageBackendId())
	if backend == nil {
		return nil, errors.Wrap(account_settings.ErrStorageBackendNotFound, placement.GetStorageBackendId())
	}
	return backend, nil
}

// runPlacedUploads keeps every placed block store mounted while the account
// runs, so writes queued in a closed Space still upload.
func (a *ProviderAccount) runPlacedUploads(ctx context.Context) error {
	ref, err := a.lookupAccountSettingsRef(ctx)
	if err != nil || ref == nil {
		return err
	}
	watch, err := a.watchAccountSettings(ctx, ref)
	if err != nil {
		return err
	}
	defer watch.release()

	held := make(map[string]*keyed.KeyedRef[string, *bstoreTracker])
	defer func() {
		for _, ref := range held {
			ref.Release()
		}
	}()
	for {
		settings, err := watch.next(ctx)
		if err != nil {
			return err
		}

		placed := make(map[string]struct{}, len(settings.GetBlockStorePlacements()))
		for _, placement := range settings.GetBlockStorePlacements() {
			placed[placement.GetBlockStoreId()] = struct{}{}
		}
		for id, ref := range held {
			if _, ok := placed[id]; !ok {
				ref.Release()
				delete(held, id)
			}
		}
		for id := range placed {
			if held[id] == nil {
				held[id], _, _ = a.bstores.AddKeyRef(id)
			}
		}
	}
}

// runUpload uploads the store's queued blocks to the backend's bucket, which
// also serves reads the local store misses.
//
// The bucket keeps serving reads across failed uploads and their retries, so
// an unreachable bucket fails reads with its own error. Clearing the placement
// cancels ctx and withdraws the bucket.
func (t *bstoreTracker) runUpload(
	ctx context.Context,
	wb *block_store_writeback.Store,
	backend *account_settings.StorageBackend,
) error {
	remote, err := t.openBackendStore(ctx, backend)
	if err != nil {
		wb.SetError(err)
		return err
	}
	t.remote.Store(&remote)
	err = wb.Upload(ctx, remote)
	if ctx.Err() != nil {
		t.remote.Store(nil)
	}
	return err
}

// openBackendStore opens the backend's bucket with its credential Secret.
func (t *bstoreTracker) openBackendStore(
	ctx context.Context,
	backend *account_settings.StorageBackend,
) (block.StoreOps, error) {
	creds, err := t.a.ReadStorageCredentials(ctx, backend)
	if err != nil {
		return nil, err
	}
	return buildS3BlockStore(backend.GetS3(), creds)
}

// getRemote returns the backend store serving reads, or nil when none is open.
func (t *bstoreTracker) getRemote() block.StoreOps {
	if remote := t.remote.Load(); remote != nil {
		return *remote
	}
	return nil
}
