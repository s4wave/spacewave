package provider_local

import (
	"context"

	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// deletingKey is the session object store key that marks the account as being
// deleted. An account delete writes it before it removes the first session
// key, and it goes with the account volume. No session mounts under it: a
// mount of a session whose key the delete removed would create a new key, and
// open a PIN-locked account without its PIN under a new identity.
var deletingKey = []byte("deleting")

// Deleting is what the deleting mark records about the delete that wrote it.
type Deleting byte

const (
	// NotDeleting means the account holds no mark.
	NotDeleting Deleting = iota
	// DeletingAccount is a delete of the account and everything it holds.
	DeletingAccount
	// DeletingTransferSource is the source delete of a Merge or Migrate, which
	// may delete the account only while it lists no Space the run did not
	// move.
	DeletingTransferSource
)

// MarkAccountDeleting writes the deleting mark, recording kind, to the
// account's session object store.
func MarkAccountDeleting(ctx context.Context, objStore object.ObjectStore, kind Deleting) error {
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, deletingKey, []byte{byte(kind)})
		},
	)
}

// ClearAccountDeleting removes the deleting mark, for a delete that stopped
// before it removed a key.
func ClearAccountDeleting(ctx context.Context, objStore object.ObjectStore) error {
	return kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Delete(ctx, deletingKey)
		},
	)
}

// AccountDeleting returns the deleting mark of the account's session object
// store, NotDeleting when it holds none.
func AccountDeleting(ctx context.Context, objStore object.ObjectStore) (Deleting, error) {
	var kind Deleting
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, deletingKey)
			kind = NotDeleting
			if found && len(data) != 0 {
				kind = Deleting(data[0])
			}
			return err
		},
	)
	return kind, err
}

// refuseDeleting returns session.ErrAccountDeleting while the account's
// session object store holds the deleting mark.
func refuseDeleting(ctx context.Context, objStore object.ObjectStore) error {
	kind, err := AccountDeleting(ctx, objStore)
	if err != nil {
		return err
	}
	if kind != NotDeleting {
		return session.ErrAccountDeleting
	}
	return nil
}

// accountVolumeConfig returns the storage volume config of the account.
func (p *Provider) accountVolumeConfig(accountID string) *storage_volume.Config {
	// Select the storage, then name the account's volume in it.
	storageID := p.storageID
	if storageID == "" {
		storageID = "default"
	}
	volumeID := StorageVolumeID(p.info.GetProviderId(), accountID)
	return &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: volumeID,
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{volumeID},
		},
	}
}

// DeleteAccountVolume removes the account's volume without opening it, so it
// never creates one; a volume already gone is no error. The volume must not
// be open. A resumed account delete removes the volume through it.
func (p *Provider) DeleteAccountVolume(ctx context.Context, accountID string) error {
	return storage_volume.DeleteVolume(ctx, p.b, p.accountVolumeConfig(accountID))
}
