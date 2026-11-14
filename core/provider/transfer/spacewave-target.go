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
	"github.com/s4wave/spacewave/net/peer"
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

// cloudCreate is the write a Space the cloud does not list takes: one
// create-with-state request with the transfer's origin, which the cloud
// refuses once it holds the id.
type cloudCreate struct {
	// meta describes the Space to create.
	meta *sobject.SharedObjectMeta
}

// cloudResync is the write over a Space this source's transfer wrote and
// nothing changed since: a checkpoint the cloud takes only while the Space's
// written head is still head.
type cloudResync struct {
	// head is the written head the cloud stored with the origin.
	head []byte
}

// cloudGenesis is the write a listed Space with no origin and no config
// takes: its genesis config, which the cloud refuses on a configured Space,
// then its checkpoint.
type cloudGenesis struct{}

// ReadSharedObject reads what the cloud holds under sharedObjectID. The cloud
// stores the origin of a transfer's write with the Space and tells whether
// anything changed the Space since. A Space with another source's origin is
// another's; one this source wrote counts as written while unchanged, or when
// it holds the source's current World. A row with no origin, from a client
// or cloud that did not store one, is judged by its state alone: a Space with
// no config is arriving, one holding the source's current World is written,
// and any other holds no origin to say whose it is.
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

	// Read the stored origin. Only a manager reads it, and a cloud without
	// origins answers 404, which reads as a row with no origin.
	stored, err := t.account.GetSessionClient().GetTransferOrigin(ctx, sharedObjectID)
	switch {
	case provider_spacewave.IsCloudErrorStatus(err, 403):
		return &TargetSpace{Holds: TargetHoldsOther}, nil
	case provider_spacewave.IsCloudErrorStatus(err, 404):
		stored = &api.GetTransferOriginResponse{}
	case err != nil:
		return nil, err
	}
	if len(stored.GetTransferOrigin()) == 0 {
		return t.readNoOrigin(ctx, le, entry.GetRef(), origin)
	}

	// Only this source's write counts, written while nothing changed it.
	src := &TransferOrigin{}
	if src.UnmarshalVT(stored.GetTransferOrigin()) != nil || !sameSource(src, origin) {
		return &TargetSpace{Holds: TargetHoldsOther}, nil
	}
	if stored.GetUnchanged() {
		return &TargetSpace{Holds: TargetHoldsWritten, Head: src.GetSourceHead(), Expect: cloudResync{head: stored.GetWrittenHead()}}, nil
	}
	return t.readCurrent(ctx, le, entry.GetRef(), origin, TargetHoldsOther)
}

// readNoOrigin judges a listed Space the cloud stores no origin for by its
// state alone.
func (t *SpacewaveTransferTarget) readNoOrigin(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef, origin *TransferOrigin) (*TargetSpace, error) {
	configured, err := t.configured(ctx, ref.GetProviderResourceRef().GetId())
	if err != nil {
		return nil, err
	}
	if !configured {
		return &TargetSpace{Holds: TargetHoldsArriving, Expect: cloudGenesis{}}, nil
	}
	return t.readCurrent(ctx, le, ref, origin, TargetHoldsNoOrigin)
}

// readCurrent returns a Space the transfer may not write over as written
// when it holds the source's current World, which needs no write, and as
// holding otherwise.
func (t *SpacewaveTransferTarget) readCurrent(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef, origin *TransferOrigin, otherwise TargetHolds) (*TargetSpace, error) {
	head, err := t.replayHead(ctx, le, ref)
	if err != nil {
		return nil, err
	}
	if head == nil || len(origin.GetSourceHead()) == 0 || !bytes.Equal(head, origin.GetSourceHead()) {
		return &TargetSpace{Holds: otherwise}, nil
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

// AddSharedObject records the Space to create with its state. The cloud
// creates the listing, config, checkpoint and origin in the one request
// WriteSharedObjectState sends, so nothing is listed before the state, and a
// listing that appears in between makes that write return ErrTargetChanged.
func (t *SpacewaveTransferTarget) AddSharedObject(_ context.Context, _ *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta, _ *TransferOrigin) (*TargetSpace, error) {
	return &TargetSpace{Holds: TargetHoldsArriving, Expect: cloudCreate{meta: meta}}, nil
}

// WriteSharedObjectState writes the Space as the read expect chose: a create
// with its state and origin, a checkpoint over the transfer's own unchanged
// write, or a genesis on a listed Space with no origin and no config. Each is
// a compare-and-set the cloud refuses when the Space changed, which returns
// ErrTargetChanged.
func (t *SpacewaveTransferTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, origin *TransferOrigin, expect *TargetSpace) error {
	var err error
	switch exp := expect.GetExpect().(type) {
	case cloudCreate:
		err = t.create(ctx, le, sharedObjectID, owner, stateData, origin, exp.meta)
	case cloudResync:
		err = t.resync(ctx, le, sharedObjectID, owner, stateData, origin, exp.head)
	case cloudGenesis:
		err = t.writeGenesis(ctx, le, sharedObjectID, owner, stateData)
	default:
		return ErrTargetChanged
	}
	if provider_spacewave.IsCloudErrorStatus(err, 409) {
		return ErrTargetChanged
	}
	return err
}

// create creates the Space with a genesis holding stateData and the
// transfer's origin, in one request.
func (t *SpacewaveTransferTarget) create(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, origin *TransferOrigin, meta *sobject.SharedObjectMeta) error {
	// Build the genesis.
	state, genesis, err := sobject.BuildGenesisSOState(le, t.account.GetStepFactorySet(), sharedObjectID, owner, stateData)
	if err != nil {
		return err
	}
	genesisData, err := genesis.MarshalVT()
	if err != nil {
		return err
	}

	// Encode the config, checkpoint and origin the request carries.
	config, err := (&api.PostConfigStateRequest{ConfigChange: genesisData, KeyEpoch: state.CurrentKeyEpoch()}).MarshalVT()
	if err != nil {
		return err
	}
	checkpoint, err := (&api.PostCheckpointRequest{Checkpoint: state.GetCheckpoint()}).MarshalVT()
	if err != nil {
		return err
	}
	src, err := origin.MarshalVT()
	if err != nil {
		return err
	}

	// Create the Space with them.
	return t.account.GetSessionClient().CreateSharedObjectWithState(ctx, sharedObjectID, &api.CreateWithStateRequest{
		ObjectType:      meta.GetBodyType(),
		OwnerType:       "account",
		OwnerId:         t.accountID,
		AccountPrivate:  meta.GetAccountPrivate(),
		ConfigState:     config,
		CheckpointState: checkpoint,
		TransferOrigin:  src,
	})
}

// resync posts a checkpoint holding stateData over the transfer's own write,
// which the cloud takes only while the Space's written head is still head.
// That write's genesis named owner alone, and nothing changed the Space
// since, so owner holds the current key epoch's grant.
func (t *SpacewaveTransferTarget) resync(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, origin *TransferOrigin, head []byte) error {
	// Read the state the checkpoint follows.
	cli := t.account.GetSessionClient()
	data, err := cli.GetSOState(ctx, sharedObjectID, 0, provider_spacewave.SeedReasonColdSeed)
	if err != nil {
		return err
	}
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "read the written Space's state")
	}
	state := msg.GetSnapshot()
	if state == nil {
		return ErrTargetChanged
	}

	// Encode stateData with the current key epoch and sign the checkpoint.
	peerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		return err
	}
	xfrm, err := sobject.NewSOStateParticipantHandle(le, t.account.GetStepFactorySet(), sharedObjectID, state, owner, peerID).GetTransformer(ctx)
	if err != nil {
		return err
	}
	enc, err := xfrm.EncodeBlock(stateData)
	if err != nil {
		return err
	}
	checkpoint, err := state.BuildNextCheckpoint(sharedObjectID, owner, enc)
	if err != nil {
		return err
	}

	// Post it with the new origin, guarded by the written head.
	src, err := origin.MarshalVT()
	if err != nil {
		return err
	}
	return cli.PostTransferCheckpoint(ctx, sharedObjectID, checkpoint, head, src)
}

// writeGenesis writes the genesis config holding stateData to a listed Space
// with no config, then its checkpoint. The cloud refuses a genesis config on
// a configured Space.
func (t *SpacewaveTransferTarget) writeGenesis(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte) error {
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
	if err := cli.PostConfigState(ctx, sharedObjectID, genesisData, nil, state.CurrentKeyEpoch(), nil); err != nil {
		return err
	}
	return cli.PostCheckpoint(ctx, sharedObjectID, state.GetCheckpoint())
}

// _ is a type assertion
var _ TransferTarget = (*SpacewaveTransferTarget)(nil)
