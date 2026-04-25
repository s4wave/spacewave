package provider_transfer

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
)

// LocalTransferTarget implements TransferTarget for a local provider account.
type LocalTransferTarget struct {
	account    *provider_local.ProviderAccount
	providerID string
	accountID  string
	b          bus.Bus
}

// NewLocalTransferTarget creates a new LocalTransferTarget.
func NewLocalTransferTarget(
	account *provider_local.ProviderAccount,
	providerID, accountID string,
	b bus.Bus,
) *LocalTransferTarget {
	return &LocalTransferTarget{
		account:    account,
		providerID: providerID,
		accountID:  accountID,
		b:          b,
	}
}

// GetAccount returns the underlying local provider account.
func (t *LocalTransferTarget) GetAccount() *provider_local.ProviderAccount {
	return t.account
}

// GetBlockStore returns the block store ops for writing blocks to the target.
// Creates the block store bucket if it does not exist.
func (t *LocalTransferTarget) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	blockStoreID := ref.GetBlockStoreId()
	if _, err := t.account.CreateBlockStore(ctx, blockStoreID); err != nil {
		// Ignore if already exists, CreateBlockStore is idempotent for local.
		_ = err
	}

	bsRef := &bstore.BlockStoreRef{
		ProviderResourceRef: ref.GetProviderResourceRef().CloneVT(),
	}
	bsRef.ProviderResourceRef.Id = blockStoreID
	// Override to target account refs.
	bsRef.ProviderResourceRef.ProviderId = t.providerID
	bsRef.ProviderResourceRef.ProviderAccountId = t.accountID

	bs, rel, err := t.account.MountBlockStore(ctx, bsRef, nil)
	if err != nil {
		return nil, nil, err
	}
	return bs, rel, nil
}

// AddSharedObject adds a shared object entry to the target's SO list.
func (t *LocalTransferTarget) AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta) error {
	soID := ref.GetProviderResourceRef().GetId()
	_, err := t.account.CreateSharedObject(ctx, soID, meta, "", "")
	if errors.Is(err, sobject.ErrSharedObjectExists) {
		return nil
	}
	return err
}

// WriteSharedObjectState writes the SO state for a shared object to the target's object store.
func (t *LocalTransferTarget) WriteSharedObjectState(ctx context.Context, sharedObjectID string, state *sobject.SOState) error {
	objStore, rel, err := t.buildObjectStore(ctx)
	if err != nil {
		return err
	}
	defer rel()

	data, err := state.MarshalVT()
	if err != nil {
		return err
	}

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	key := provider_local.SobjectObjectStoreHostStateKey(sharedObjectID)
	if err := otx.Set(ctx, key, data); err != nil {
		return err
	}
	return otx.Commit(ctx)
}

// buildObjectStore builds an object store handle for the target account.
func (t *LocalTransferTarget) buildObjectStore(ctx context.Context) (object.ObjectStore, func(), error) {
	objStoreID := provider_local.SobjectObjectStoreID(t.providerID, t.accountID)
	volID := t.account.GetVolume().GetID()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, t.b, false, objStoreID, volID, nil)
	if err != nil {
		return nil, nil, err
	}
	return handle.GetObjectStore(), diRef.Release, nil
}

// _ is a type assertion
var _ TransferTarget = (*LocalTransferTarget)(nil)
