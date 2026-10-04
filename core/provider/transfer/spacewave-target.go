package provider_transfer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
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
	// Ensure the cloud account has a block-store bucket for the shared object.
	blockStoreID := ref.GetBlockStoreId()
	if _, err := t.account.CreateBlockStore(ctx, blockStoreID); err != nil &&
		!errors.Is(err, bstore.ErrBlockStoreExists) &&
		!provider_spacewave.IsCloudErrorStatus(err, 409) {
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

// AddSharedObject creates a shared object on the cloud.
// Only creates the container; state is written separately via WriteSharedObjectState.
func (t *SpacewaveTransferTarget) AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta) error {
	// Create the cloud shared object, treating an existing object as complete.
	soID := ref.GetProviderResourceRef().GetId()
	cli := t.account.GetSessionClient()
	err := cli.CreateSharedObject(ctx, soID, "", meta.GetBodyType(), "", "", meta.GetAccountPrivate())
	if provider_spacewave.IsCloudErrorStatus(err, 409) {
		return nil
	}
	return err
}

// WriteSharedObjectState initializes the cloud shared object with the
// genesis config and key epoch, then its checkpoint.
func (t *SpacewaveTransferTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte) error {
	// Build the genesis, then post its config and checkpoint.
	state, genesis, err := sobject.BuildGenesisSOState(le, t.account.GetStepFactorySet(), sharedObjectID, owner, stateData)
	if err != nil {
		return err
	}
	genesisData, err := genesis.MarshalVT()
	if err != nil {
		return err
	}
	cli := t.account.GetSessionClient()
	if err := cli.PostConfigState(ctx, sharedObjectID, genesisData, nil, state.CurrentKeyEpoch(), nil); err != nil {
		return err
	}
	return cli.PostCheckpoint(ctx, sharedObjectID, state.GetCheckpoint())
}

// _ is a type assertion
var _ TransferTarget = (*SpacewaveTransferTarget)(nil)
