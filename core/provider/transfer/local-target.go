package provider_transfer

import (
	"bytes"
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

// ReadSharedObject reads what the account stores under sharedObjectID. A
// written Space whose state hash no longer matches is replayed. It counts as
// written when its World is the source's current one, since no write follows;
// or when its World is the one the transfer copied, no config change or
// invite was added since, and the stored state stays put across the replay,
// since a write then replaces only that World.
func (t *LocalTransferTarget) ReadSharedObject(ctx context.Context, le *logrus.Entry, sharedObjectID string, origin *TransferOrigin) (*TargetSpace, error) {
	// Read the listing, origin and state together.
	stored, err := t.account.ReadStoredSharedObject(ctx, sharedObjectID)
	if err != nil {
		return nil, err
	}
	switch {
	case stored.Entry == nil:
		return &TargetSpace{Holds: TargetHoldsNothing, Expect: stored}, nil
	case stored.Entry.GetSource() == "shared":
		return &TargetSpace{Holds: TargetHoldsShared, Expect: stored}, nil
	}

	// Only a transfer from this source counts.
	src := &TransferOrigin{}
	if stored.Origin == nil || src.UnmarshalVT(stored.Origin.GetSource()) != nil || !sameSource(src, origin) {
		return &TargetSpace{Holds: TargetHoldsOther, Expect: stored}, nil
	}
	if stored.Origin.GetArriving() {
		return &TargetSpace{Holds: TargetHoldsArriving, Expect: stored}, nil
	}
	written := &TargetSpace{Holds: TargetHoldsWritten, Head: src.GetSourceHead(), Expect: stored}
	if stored.Unchanged() {
		return written, nil
	}

	// The state changed. A target holding the source's current World needs
	// no write, whatever else changed.
	head, err := t.replayHead(ctx, le, stored.Entry.GetRef())
	if err != nil {
		return nil, err
	}
	if len(origin.GetSourceHead()) != 0 && bytes.Equal(head, origin.GetSourceHead()) {
		return &TargetSpace{Holds: TargetHoldsWritten, Head: head, Expect: stored}, nil
	}

	// A write replaces the whole state, so it may follow only while the World
	// is the one copied, the membership is the write's, and the state stays
	// put across the replay.
	again, err := t.account.ReadStoredSharedObject(ctx, sharedObjectID)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(head, src.GetSourceHead()) || membershipChanged(stored.State) || again.Entry.GetGeneration() != stored.Entry.GetGeneration() || !bytes.Equal(again.State, stored.State) {
		return &TargetSpace{Holds: TargetHoldsOther, Expect: stored}, nil
	}
	return written, nil
}

// membershipChanged reports whether a stored state has a config change or
// an invite the transfer's write did not make. The write starts a lineage at
// its genesis config, which no change has followed, with no invites; a write
// over a later state would drop the participants and invites added since.
func membershipChanged(state []byte) bool {
	st := &sobject.SOState{}
	if err := st.UnmarshalVT(state); err != nil {
		return true
	}
	return st.GetConfig().GetConfigChainSeqno() != 0 || len(st.GetInvites()) != 0
}

// replayHead returns the SHA-256 of the World the target's Space at ref
// replays.
func (t *LocalTransferTarget) replayHead(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) ([]byte, error) {
	world, release, err := replaySharedObject(ctx, le, t.b, t.account.GetStepFactorySet(), t.account, ref)
	if err != nil {
		return nil, err
	}
	defer release()
	return worldHead(world)
}

// AddSharedObject lists the Space arriving from origin and reads what the
// account then holds.
func (t *LocalTransferTarget) AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta, origin *TransferOrigin) (*TargetSpace, error) {
	// List the Space with its origin.
	soID := ref.GetProviderResourceRef().GetId()
	src, err := origin.MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := t.account.CreateArrivingSharedObject(ctx, soID, meta, src); err != nil {
		return nil, err
	}

	// Read the arriving Space back for the write's compare-and-set.
	stored, err := t.account.ReadStoredSharedObject(ctx, soID)
	if err != nil {
		return nil, err
	}
	return &TargetSpace{Holds: TargetHoldsArriving, Expect: stored}, nil
}

// WriteSharedObjectState replaces the state and config history of the Space
// and records origin as its source, in one transaction that requires the
// generation and state expect read.
func (t *LocalTransferTarget) WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte, origin *TransferOrigin, expect *TargetSpace) error {
	// Write only over a listed Space this target read.
	stored, ok := expect.GetExpect().(*provider_local.StoredSharedObject)
	if !ok || stored.Entry == nil {
		return ErrTargetChanged
	}

	// Replace the state the read saw, recording origin as its source.
	src, err := origin.MarshalVT()
	if err != nil {
		return err
	}
	err = t.account.ReplaceSharedObjectState(ctx, le, sharedObjectID, owner, stateData, provider_local.ReplaceOrigin{
		Generation: stored.Entry.GetGeneration(),
		State:      stored.State,
		Source:     src,
	})
	if errors.Is(err, provider_local.ErrStateChanged) || errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return ErrTargetChanged
	}
	return err
}

// _ is a type assertion
var _ TransferTarget = (*LocalTransferTarget)(nil)
