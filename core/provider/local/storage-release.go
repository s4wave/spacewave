package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// storageRelease names a block store whose objects remain on a storage
// backend it left.
type storageRelease struct {
	blockStoreID string
	backendID    string
}

// newStorageReleases builds the routines that delete released block stores
// from their backends' buckets, retrying failures with backoff.
func (a *ProviderAccount) newStorageReleases() *keyed.Keyed[storageRelease, struct{}] {
	return keyed.NewKeyedWithLogger(
		func(release storageRelease) (keyed.Routine, struct{}) {
			return func(ctx context.Context) error {
				return a.releaseBlockStore(ctx, release)
			}, struct{}{}
		},
		a.le.WithField("subsystem", "storage-releases"),
		keyed.WithRetry[storageRelease, struct{}](&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		}),
	)
}

// pendingStorageReleases returns the releases the settings record.
func pendingStorageReleases(settings *account_settings.AccountSettings) []storageRelease {
	releases := make([]storageRelease, len(settings.GetStorageReleases()))
	for i, release := range settings.GetStorageReleases() {
		releases[i] = storageRelease{
			blockStoreID: release.GetBlockStoreId(),
			backendID:    release.GetStorageBackendId(),
		}
	}
	return releases
}

// releaseBlockStore deletes a released block store's objects from the
// backend's bucket, then completes the release in the account settings.
//
// The settings cancel a release when the store returns to the backend, and
// the routine stops when its release leaves the settings. A device that
// returns the store while another device deletes can still lose the objects
// it uploads meanwhile; its local copy remains.
func (a *ProviderAccount) releaseBlockStore(ctx context.Context, release storageRelease) error {
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	backend := settings.FindStorageBackend(release.backendID)
	if backend == nil {
		return nil
	}
	creds, err := a.ReadStorageCredentials(ctx, backend)
	if err != nil {
		return err
	}
	if err := deleteS3BlockStore(ctx, backend.GetS3(), release.blockStoreID, creds); err != nil {
		return err
	}
	return a.commitAccountSettingsOps(ctx, &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_CompleteStorageRelease{
			CompleteStorageRelease: &account_settings.BlockStorePlacement{
				BlockStoreId:     release.blockStoreID,
				StorageBackendId: release.backendID,
			},
		},
	})
}
