package provider_spacewave

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// linkedLocalKey returns the ObjectStore key for the linked-local session index.
func linkedLocalKey(sessionID string) []byte {
	return []byte(sessionID + "/linked-local")
}

// GetLinkedLocalSession reads the linked-local session index from the ObjectStore.
func (a *ProviderAccount) GetLinkedLocalSession(ctx context.Context, sessionID string) (bool, uint32, error) {
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(a.accountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return false, 0, errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	var idx uint32
	var found bool
	objStore := objStoreHandle.GetObjectStore()
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, attemptFound, err := tx.Get(ctx, linkedLocalKey(sessionID))
			if err != nil {
				return errors.Wrap(err, "read linked-local key")
			}
			if !attemptFound {
				found = false
				return nil
			}
			attemptIdx, err := strconv.ParseUint(string(data), 10, 32)
			if err != nil {
				return errors.Wrap(err, "parse linked-local index")
			}
			found = true
			idx = uint32(attemptIdx)
			return nil
		},
	)
	if err != nil {
		return false, 0, errors.Wrap(err, "new read transaction")
	}
	if !found {
		return false, 0, nil
	}

	// Verify the local session still exists (resilient to stale refs).
	sessionCtrl, sessionCtrlRef, serr := session.ExLookupSessionController(ctx, a.p.b, "", false, nil)
	if serr == nil {
		defer sessionCtrlRef.Release()
		entry, gerr := sessionCtrl.GetSessionByIdx(ctx, idx)
		if gerr == nil && entry == nil {
			// Session index is stale, best-effort cleanup.
			_ = a.DeleteLinkedLocalSession(ctx, sessionID)
			return false, 0, nil
		}
	}
	return true, idx, nil
}

// DeleteLinkedLocalSession removes the linked-local session key from the ObjectStore.
func (a *ProviderAccount) DeleteLinkedLocalSession(ctx context.Context, sessionID string) error {
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(a.accountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	objStore := objStoreHandle.GetObjectStore()
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Delete(ctx, linkedLocalKey(sessionID)); err != nil {
				return errors.Wrap(err, "delete linked-local key")
			}
			return nil
		},
	)
	return errors.Wrap(err, "new write transaction")
}

// SetLinkedLocalSession writes the linked-local session index to the ObjectStore.
func (a *ProviderAccount) SetLinkedLocalSession(ctx context.Context, sessionID string, localIdx uint32) error {
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(a.accountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	val := []byte(strconv.FormatUint(uint64(localIdx), 10))
	objStore := objStoreHandle.GetObjectStore()
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, linkedLocalKey(sessionID), val); err != nil {
				return errors.Wrap(err, "set linked-local key")
			}
			return nil
		},
	)
	return errors.Wrap(err, "new write transaction")
}
