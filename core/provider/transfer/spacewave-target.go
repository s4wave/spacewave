package provider_transfer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// SpacewaveTransferTarget implements TransferTarget for a spacewave cloud account.
type SpacewaveTransferTarget struct {
	account    *provider_spacewave.ProviderAccount
	providerID string
	accountID  string
}

// NewSpacewaveTransferTarget creates a new SpacewaveTransferTarget.
func NewSpacewaveTransferTarget(
	account *provider_spacewave.ProviderAccount,
	providerID, accountID string,
) *SpacewaveTransferTarget {
	return &SpacewaveTransferTarget{
		account:    account,
		providerID: providerID,
		accountID:  accountID,
	}
}

// GetAccount returns the underlying spacewave provider account.
func (t *SpacewaveTransferTarget) GetAccount() *provider_spacewave.ProviderAccount {
	return t.account
}

// GetBlockStore returns the block store ops for writing blocks to the target.
// Creates the block store if it does not exist.
func (t *SpacewaveTransferTarget) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	blockStoreID := ref.GetBlockStoreId()
	if _, err := t.account.CreateBlockStore(ctx, blockStoreID); err != nil {
		_ = err
	}

	bsRef := &bstore.BlockStoreRef{
		ProviderResourceRef: ref.GetProviderResourceRef().CloneVT(),
	}
	bsRef.ProviderResourceRef.Id = blockStoreID
	bsRef.ProviderResourceRef.ProviderId = t.providerID
	bsRef.ProviderResourceRef.ProviderAccountId = t.accountID

	bs, rel, err := t.account.MountBlockStore(ctx, bsRef, nil)
	if err != nil {
		return nil, nil, err
	}
	return bs, rel, nil
}

// AddSharedObject creates a shared object on the cloud.
// Only creates the container; state is written separately via WriteSharedObjectState.
func (t *SpacewaveTransferTarget) AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta) error {
	soID := ref.GetProviderResourceRef().GetId()
	cli := t.account.GetSessionClient()
	err := cli.CreateSharedObject(ctx, soID, "", meta.GetBodyType(), "", "", meta.GetAccountPrivate())
	if provider_spacewave.IsCloudErrorStatus(err, 409) {
		return nil
	}
	return err
}

// WriteSharedObjectState writes the SO state to the cloud.
func (t *SpacewaveTransferTarget) WriteSharedObjectState(ctx context.Context, sharedObjectID string, state *sobject.SOState) error {
	if state == nil || state.GetRoot() == nil {
		return errors.New("shared object root is required")
	}

	data, err := state.GetRoot().MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal SO root")
	}
	cli := t.account.GetSessionClient()
	return cli.PostInitState(ctx, sharedObjectID, data)
}

// _ is a type assertion
var _ TransferTarget = (*SpacewaveTransferTarget)(nil)
