package provider_transfer

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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
// cloud records no origin, so a configured Space counts as written only when
// its World is the source's current one, which needs no write; anything else
// configured is held for another reason, and the transfer never writes it. A
// listed Space with no config is arriving, since a write to it loses nothing
// and the cloud refuses a second genesis.
func (t *SpacewaveTransferTarget) ReadSharedObject(ctx context.Context, le *logrus.Entry, sharedObjectID string, origin *TransferOrigin) (*TargetSpace, error) {
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

	// A Space with no config is still arriving.
	configured, err := t.configured(ctx, sharedObjectID)
	if err != nil {
		return nil, err
	}
	if !configured {
		return &TargetSpace{Holds: TargetHoldsArriving}, nil
	}

	// A configured Space holds the source's current World or is another's.
	head, err := t.replayHead(ctx, le, entry.GetRef())
	if err != nil {
		return nil, err
	}
	if head == nil || len(origin.GetSourceHead()) == 0 || !bytes.Equal(head, origin.GetSourceHead()) {
		return &TargetSpace{Holds: TargetHoldsOther}, nil
	}
	return &TargetSpace{Holds: TargetHoldsWritten, Head: head}, nil
}

// configured reports whether the cloud holds a config chain for the Space.
func (t *SpacewaveTransferTarget) configured(ctx context.Context, sharedObjectID string) (bool, error) {
	// Fetch the Space's config chain from the cloud.
	data, err := t.account.GetSessionClient().GetConfigChain(ctx, sharedObjectID)
	if err != nil {
		return false, errors.Wrap(err, "read the listed Space's config")
	}

	// Decode it and report whether it holds any change.
	chain := &sobject.SOConfigChainResponse{}
	if err := chain.UnmarshalVT(data); err != nil {
		return false, errors.Wrap(err, "read the listed Space's config")
	}
	return len(chain.GetConfigChanges()) != 0, nil
}

// replayHead returns the head of the World the cloud's Space at ref replays,
// nil when the cloud holds its config but no checkpoint, as after a write
// that stopped between the two.
func (t *SpacewaveTransferTarget) replayHead(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) ([]byte, error) {
	// Read the state the cloud holds, refusing to mount a Space without a
	// checkpoint to replay.
	soID := ref.GetProviderResourceRef().GetId()
	data, err := t.account.GetSessionClient().GetSOState(ctx, soID, 0, provider_spacewave.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "read the listed Space's state")
	}
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "read the listed Space's state")
	}
	if msg.GetSnapshot().GetCheckpoint().SizeVT() == 0 {
		return nil, nil
	}

	// Replay its World.
	world, release, err := replaySharedObject(ctx, le, t.account.SpaceBus(), t.account.GetStepFactorySet(), t.account, ref)
	if err != nil {
		return nil, errors.Wrap(err, "replay the listed Space")
	}
	defer release()
	return worldHead(world)
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
