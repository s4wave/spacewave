package provider_local

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// spaceBackupRegistryStore opens the account's device-local configuration view.
// These rows are outside every SharedObject and never synchronize to a Space.
func (a *ProviderAccount) spaceBackupRegistryStore() (kvtx.Store, error) {
	// A local volume exposes its transactional storage, without opening a writer.
	vol, ok := a.vol.(volume_kvtx.KvtxVolume)
	if !ok {
		return nil, errors.New("this device's volume has no local backup state store")
	}
	return kvtx_prefixer.NewPrefixer(vol.GetKvtxStore(), []byte("account-space-backups/"+a.GetProviderID()+"/"+a.GetAccountID()+"/")), nil
}

// loadSpaceBackups starts the saved local backup routines when the account opens.
func (a *ProviderAccount) loadSpaceBackups(ctx context.Context) error {
	// Read each small configuration without retaining the transaction across mounts.
	store, err := a.spaceBackupRegistryStore()
	if err != nil {
		return err
	}
	var after []byte
	for {
		conf, next, err := nextSpaceBackupConfig(ctx, store, after)
		if err != nil || conf == nil {
			return err
		}
		after = next

		// Publish one configuration without keeping its read transaction open.
		unlock, err := a.backupsConfigMtx.Lock(ctx)
		if err != nil {
			return err
		}
		var existing *SpaceBackup
		a.backupsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { existing = a.backups[conf.GetId()] })
		if existing == nil {
			err = a.installSpaceBackup(ctx, conf)
		}
		unlock()
		if err != nil {
			return err
		}
	}
}

// nextSpaceBackupConfig reads one indexed configuration and releases its snapshot.
func nextSpaceBackupConfig(ctx context.Context, store kvtx.Store, after []byte) (*account_settings.SpaceBackup, []byte, error) {
	// Seek strictly after the previous key with no retained cursor or transaction.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, []byte("config/"), true, false)
	defer it.Close()
	if after == nil {
		it.Next()
	} else if err := it.Seek(append(bytes.Clone(after), 0)); err != nil {
		return nil, nil, err
	}
	if !it.Valid() {
		return nil, nil, it.Err()
	}

	// Decode just this row before returning a copied continuation key.
	data, err := it.Value()
	if err != nil {
		return nil, nil, err
	}
	conf := &account_settings.SpaceBackup{}
	if err := conf.UnmarshalVT(data); err != nil {
		return nil, nil, err
	}
	return conf, bytes.Clone(it.Key()), nil
}

// findSpaceBackupTarget streams local configurations until the target matches.
// Each indexed continuation read ends before inspecting the next configuration.
func findSpaceBackupTarget(ctx context.Context, store kvtx.Store, spaceID, backendID string) (*account_settings.SpaceBackup, error) {
	var after []byte
	for {
		conf, next, err := nextSpaceBackupConfig(ctx, store, after)
		if err != nil || conf == nil {
			return nil, err
		}
		if conf.GetSpaceId() == spaceID && conf.GetStorageBackendId() == backendID {
			return conf, nil
		}
		after = next
	}
}

// SetSpaceBackup saves a complete device-local backup configuration. A new
// backup always starts with automatic work off; a later explicit set enables it.
func (a *ProviderAccount) SetSpaceBackup(ctx context.Context, conf *account_settings.SpaceBackup) (*account_settings.SpaceBackup, error) {
	// Resolve the account's existing backend, never a primary storage placement.
	if conf.GetSpaceId() == "" || conf.GetStorageBackendId() == "" {
		return nil, errors.New("backup Space and storage backend are required")
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}
	backend := settings.FindStorageBackend(conf.GetStorageBackendId())
	if backend == nil || backend.GetGdrive() == nil {
		return nil, errors.New("backup requires an existing Google Drive backend")
	}
	conf = conf.CloneVT()
	if conf.GetIntervalSeconds() == 0 {
		conf.IntervalSeconds = uint64(time.Hour / time.Second)
	}
	if conf.GetIntervalSeconds() > uint64((1<<63-1)/int64(time.Second)) {
		return nil, errors.New("backup interval is too long")
	}

	// Serialize only the bounded local commit and its in-memory publication.
	unlock, err := a.backupsConfigMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Reject a different source or destination on an existing proof identity.
	var previous *SpaceBackup
	a.backupsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { previous = a.backups[conf.GetId()] })
	if previous != nil {
		old := previous.currentState().GetBackup()
		if old.GetSpaceId() != conf.GetSpaceId() || old.GetStorageBackendId() != conf.GetStorageBackendId() {
			return nil, errors.New("create a new backup when changing its Space or destination")
		}
	}
	store, err := a.spaceBackupRegistryStore()
	if err != nil {
		return nil, err
	}

	// Repeating add reconciles the same target instead of starting a second actor.
	if conf.GetId() == "" {
		old, err := findSpaceBackupTarget(ctx, store, conf.GetSpaceId(), conf.GetStorageBackendId())
		if err != nil {
			return nil, err
		}
		if old != nil {
			if err := a.installSpaceBackup(ctx, old); err != nil {
				return nil, err
			}
			return old, nil
		}
		conf.Id, conf.Automatic = ulid.NewULID(), false
	}

	// A saved identity may still be loading after startup; read its existing target.
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	savedData, found, readErr := read.Get(ctx, []byte("config/"+conf.GetId()))
	old := &account_settings.SpaceBackup{}
	if readErr == nil && found {
		readErr = old.UnmarshalVT(savedData)
	}
	read.Discard()

	// Preserve the existing source and destination of this opaque backup ID.
	if readErr != nil {
		return nil, readErr
	}
	if found && (old.GetSpaceId() != conf.GetSpaceId() || old.GetStorageBackendId() != conf.GetStorageBackendId()) {
		return nil, errors.New("create a new backup when changing its Space or destination")
	}
	if !found {
		conf.Automatic = false
	}

	// Encode before opening the short configuration write.
	data, err := conf.MarshalVT()
	if err != nil {
		return nil, err
	}
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	if err := kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, []byte("config/"+conf.GetId()), data)
	}); err != nil {
		return nil, err
	}

	// Publish only committed configuration and wake its existing routine.
	if err := a.installSpaceBackup(ctx, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

// installSpaceBackup publishes a saved configuration and its stable routine.
// Its caller serializes configuration installation with backupsConfigMtx.
func (a *ProviderAccount) installSpaceBackup(ctx context.Context, conf *account_settings.SpaceBackup) error {
	// Read a previous terminal state before installing a newly loaded actor.
	var backup *SpaceBackup
	a.backupsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { backup = a.backups[conf.GetId()] })
	if backup == nil {
		previous, err := a.readSpaceBackupState(ctx, conf.GetId())
		if err != nil {
			return err
		}
		backup = newSpaceBackup(conf, previous, func() {
			a.backupsBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
		}, a.runMountedBackup)
	}

	// Keep one routine per ID while allowing configuration changes to restart it.
	a.backupsBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.backups[conf.GetId()] = backup
		broadcast()
	})
	backup.configure(a.lifecycleCtx, conf)
	return nil
}

// readSpaceBackupState reads one device-local terminal outcome without mounting a Space.
func (a *ProviderAccount) readSpaceBackupState(ctx context.Context, id string) (*account_settings.SpaceBackupState, error) {
	// Release the snapshot before the caller starts a routine or opens another writer.
	store, err := a.spaceBackupRegistryStore()
	if err != nil {
		return nil, err
	}

	// Read one terminal outcome and release its snapshot before mounting anything.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte("state/"+id))
	if err != nil || !found {
		return nil, err
	}
	state := &account_settings.SpaceBackupState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return state, nil
}

// RunSpaceBackup requests one pass and follows the routine's watched state.
// Losing the client stream does not cancel the account-owned pass.
func (a *ProviderAccount) RunSpaceBackup(ctx context.Context, id string, emit func(*account_settings.SpaceBackupState) error) error {
	// Resolve saved configuration before deciding that an ID is absent after restart.
	if err := a.backupsLoader.WaitExited(ctx, false, nil); err != nil {
		return err
	}

	// Join an already requested pass or request the next sequence.
	var backup *SpaceBackup
	a.backupsBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { backup = a.backups[id] })
	if backup == nil {
		return errors.New("backup not found; run space backup add first")
	}
	sequence := backup.request(true)
	var previous *account_settings.SpaceBackupState
	for {
		state, err := backup.waitState(ctx, previous)
		if err != nil {
			return err
		}
		previous = state
		if state.GetRunSequence() < sequence {
			continue
		}
		if err := emit(state.CloneVT()); err != nil {
			return err
		}
		if state.GetCopying() == nil {
			return nil
		}
	}
}

// WatchSpaceBackups sends the complete registry once, then only changed entries
// keyed by backup ID. A new stream always starts with a replacement snapshot.
func (a *ProviderAccount) WatchSpaceBackups(ctx context.Context, emit func(*account_settings.SpaceBackupList) error) error {
	// A partially loaded registry must not look like a complete empty snapshot.
	if err := a.backupsLoader.WaitExited(ctx, false, nil); err != nil {
		return err
	}
	previous := make(map[string]*account_settings.SpaceBackupState)
	first := true
	for {
		// Capture states and the notification in the same guarded snapshot.
		var changed <-chan struct{}
		result := &account_settings.SpaceBackupList{}
		a.backupsBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			changed = getWaitCh()
			for id, backup := range a.backups {
				state := backup.currentState()
				if previous[id] != state {
					result.Backups = append(result.Backups, state.CloneVT())
					previous[id] = state
				}
			}
		})
		slices.SortFunc(result.Backups, func(a, b *account_settings.SpaceBackupState) int {
			if a.GetBackup().GetId() < b.GetBackup().GetId() {
				return -1
			}
			if a.GetBackup().GetId() > b.GetBackup().GetId() {
				return 1
			}
			return 0
		})
		if first || len(result.GetBackups()) != 0 {
			if err := emit(result); err != nil {
				return err
			}
			first = false
		}

		// Wait for a configuration or progress event, never a periodic scan.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}
