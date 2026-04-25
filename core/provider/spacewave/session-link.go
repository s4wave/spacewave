package provider_spacewave

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
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

	objStore := objStoreHandle.GetObjectStore()
	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return false, 0, errors.Wrap(err, "new read transaction")
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, linkedLocalKey(sessionID))
	if err != nil {
		return false, 0, errors.Wrap(err, "read linked-local key")
	}
	if !found {
		return false, 0, nil
	}

	idx, err := strconv.ParseUint(string(data), 10, 32)
	if err != nil {
		return false, 0, errors.Wrap(err, "parse linked-local index")
	}

	// Verify the local session still exists (resilient to stale refs).
	sessionCtrl, sessionCtrlRef, serr := session.ExLookupSessionController(ctx, a.p.b, "", false, nil)
	if serr == nil {
		defer sessionCtrlRef.Release()
		entry, gerr := sessionCtrl.GetSessionByIdx(ctx, uint32(idx))
		if gerr == nil && entry == nil {
			// Session index is stale, best-effort cleanup.
			_ = a.DeleteLinkedLocalSession(ctx, sessionID)
			return false, 0, nil
		}
	}

	return true, uint32(idx), nil
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
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new write transaction")
	}
	defer otx.Discard()

	if err := otx.Delete(ctx, linkedLocalKey(sessionID)); err != nil {
		return errors.Wrap(err, "delete linked-local key")
	}
	return otx.Commit(ctx)
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

	objStore := objStoreHandle.GetObjectStore()
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new write transaction")
	}
	defer otx.Discard()

	val := []byte(strconv.FormatUint(uint64(localIdx), 10))
	if err := otx.Set(ctx, linkedLocalKey(sessionID), val); err != nil {
		return errors.Wrap(err, "set linked-local key")
	}
	return otx.Commit(ctx)
}
