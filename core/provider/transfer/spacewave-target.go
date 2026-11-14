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

// ReadSharedObject reads what the cloud holds under sharedObjectID. The
// cloud records no origin, so it cannot tell a Space a transfer wrote from
// any other: a listed Space with no config is arriving, since a write to it
// loses nothing and the cloud refuses a second genesis, and one with config
// is held for another reason, so the transfer never writes it and cleans
// nothing up.
func (t *SpacewaveTransferTarget) ReadSharedObject(ctx context.Context, _ *logrus.Entry, sharedObjectID string, _ *TransferOrigin) (*TargetSpace, error) {
	// Read the account's list of Spaces.
	ctr, rel, err := t.account.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	list, err := ctr.WaitValue(ctx, nil)
	rel()
	if err != nil {
		return nil, err
	}

	// Find the Space and judge it by how the cloud lists it.
	var entry *sobject.SharedObjectListEntry
	for _, e := range list.GetSharedObjects() {
		if e.GetRef().GetProviderResourceRef().GetId() == sharedObjectID {
			entry = e
			break
		}
	}
	switch {
	case entry == nil:
		return &TargetSpace{Holds: TargetHoldsNothing}, nil
	case entry.GetSource() == "shared":
		return &TargetSpace{Holds: TargetHoldsShared}, nil
	}
	return t.readConfig(ctx, sharedObjectID)
}

// readConfig judges a listed Space by its config chain alone.
func (t *SpacewaveTransferTarget) readConfig(ctx context.Context, sharedObjectID string) (*TargetSpace, error) {
	// Read the Space's config chain.
	data, err := t.account.GetSessionClient().GetConfigChain(ctx, sharedObjectID)
	if err != nil {
		return nil, errors.Wrap(err, "read the listed Space's config")
	}
	chain := &sobject.SOConfigChainResponse{}
	if err := chain.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "read the listed Space's config")
	}

	// A Space with no config is still arriving; any config is another write.
	if len(chain.GetConfigChanges()) != 0 {
		return &TargetSpace{Holds: TargetHoldsOther}, nil
	}
	return &TargetSpace{Holds: TargetHoldsArriving}, nil
}

// AddSharedObject creates the Space on the cloud. The cloud lists it with
// no config. A Space the cloud lists already returns
// sobject.ErrSharedObjectExists.
func (t *SpacewaveTransferTarget) AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta, _ *TransferOrigin) (*TargetSpace, error) {
	// Create the Space, refusing an id the cloud lists.
	soID := ref.GetProviderResourceRef().GetId()
	err := t.account.GetSessionClient().CreateSharedObject(ctx, soID, "", meta.GetBodyType(), "", "", meta.GetAccountPrivate())
	if provider_spacewave.IsCloudErrorStatus(err, 409) {
		return nil, sobject.ErrSharedObjectExists
	}
	if err != nil {
		return nil, err
	}
	return &TargetSpace{Holds: TargetHoldsArriving}, nil
}

// WriteSharedObjectState initializes the cloud Space with the genesis config
// and key epoch, then its checkpoint. The cloud refuses a genesis config on a
// Space that has one, which this returns as ErrTargetChanged, so the write
// is a compare-and-set on a Space with no config.
func (t *SpacewaveTransferTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, _ *TransferOrigin, _ *TargetSpace) error {
	// Build the genesis.
	state, genesis, err := sobject.BuildGenesisSOState(le, t.account.GetStepFactorySet(), sharedObjectID, owner, stateData)
	if err != nil {
		return err
	}
	genesisData, err := genesis.MarshalVT()
	if err != nil {
		return err
	}

	// Post its config, which the cloud refuses on a configured Space, then
	// its checkpoint.
	cli := t.account.GetSessionClient()
	err = cli.PostConfigState(ctx, sharedObjectID, genesisData, nil, state.CurrentKeyEpoch(), nil)
	if provider_spacewave.IsCloudErrorStatus(err, 409) {
		return ErrTargetChanged
	}
	if err != nil {
		return err
	}
	return cli.PostCheckpoint(ctx, sharedObjectID, state.GetCheckpoint())
}

// _ is a type assertion
var _ TransferTarget = (*SpacewaveTransferTarget)(nil)
