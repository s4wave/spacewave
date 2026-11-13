package provider_transfer

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
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
	// Ensure the target account has a block-store bucket for the shared object.
	blockStoreID := ref.GetBlockStoreId()
	if _, err := t.account.CreateBlockStore(ctx, blockStoreID); err != nil && !errors.Is(err, bstore.ErrBlockStoreExists) {
		return nil, nil, err
	}

	// Retarget the block-store reference to the target account.
	bsRef := &bstore.BlockStoreRef{
		ProviderResourceRef: ref.GetProviderResourceRef().CloneVT(),
	}
	bsRef.ProviderResourceRef.Id = blockStoreID
	bsRef.ProviderResourceRef.ProviderId = t.providerID
	bsRef.ProviderResourceRef.ProviderAccountId = t.accountID

	// Mount the target block store and return its release function.
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

// WriteSharedObjectState replaces the state and config history of a shared
// object the target lists.
func (t *LocalTransferTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte) error {
	return t.account.ReplaceSharedObjectState(ctx, le, sharedObjectID, owner, stateData)
}

// _ is a type assertion
var _ TransferTarget = (*LocalTransferTarget)(nil)
