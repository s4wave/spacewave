package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// LocalTransferSource implements TransferSource for a local provider account.
type LocalTransferSource struct {
	account *provider_local.ProviderAccount
	b       bus.Bus
}

// NewLocalTransferSource creates a new LocalTransferSource.
func NewLocalTransferSource(
	account *provider_local.ProviderAccount,
	b bus.Bus,
) *LocalTransferSource {
	return &LocalTransferSource{account: account, b: b}
}

// GetAccount returns the underlying local provider account.
func (s *LocalTransferSource) GetAccount() *provider_local.ProviderAccount {
	return s.account
}

// GetOrigin returns the origin naming the source account.
func (s *LocalTransferSource) GetOrigin() *TransferOrigin {
	return &TransferOrigin{SourceProviderId: s.account.GetProviderID(), SourceAccountId: s.account.GetAccountID()}
}

// LeavesSharedObjects returns true: a local delete removes the Space from
// this account only and sends no leave, so other members keep reading it.
func (s *LocalTransferSource) LeavesSharedObjects() bool {
	return true
}

// GetSharedObjectList returns the list of shared objects on the source account.
func (s *LocalTransferSource) GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	// Open the local provider account's shared-object list.
	ctr, rel, err := s.account.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rel()

	// Read a stable snapshot of the local shared-object list.
	val, err := ctr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return val.CloneVT(), nil
}

// ReplaySharedObject returns the World of a Space after its last operation.
func (s *LocalTransferSource) ReplaySharedObject(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) (*space_world.InnerState, func(), error) {
	return replaySharedObject(ctx, le, s.b, s.account.GetStepFactorySet(), s.account, ref)
}

// GetBlockStore returns the block store ops for reading blocks from a shared object.
func (s *LocalTransferSource) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	// Build a block-store reference for the source shared object.
	bsRef := &bstore.BlockStoreRef{
		ProviderResourceRef: ref.GetProviderResourceRef().CloneVT(),
	}
	bsRef.ProviderResourceRef.Id = ref.GetBlockStoreId()
	bs, rel, err := s.account.MountBlockStore(ctx, bsRef, nil)
	if err != nil {
		return nil, nil, err
	}
	return bs, rel, nil
}

// DeleteSharedObject deletes a shared object from the source account.
func (s *LocalTransferSource) DeleteSharedObject(ctx context.Context, soID string) error {
	return s.account.DeleteSharedObject(ctx, soID)
}

// AuthorHead returns the account's volume peer, which signs every operation
// the account writes, and the nonce of its latest operation on the Space soID
// that the account stores, 0 when it stores none.
func (s *LocalTransferSource) AuthorHead(ctx context.Context, soID string) (string, uint64, error) {
	// Name the peer the account signs as.
	p, err := s.account.GetVolume().GetPeer(ctx, true)
	if err != nil {
		return "", 0, err
	}
	peerID := p.GetPeerID().String()

	// Find its chain in the stored state.
	stored, err := s.account.ReadStoredSharedObject(ctx, soID)
	if err != nil || stored.State == nil {
		return peerID, 0, err
	}
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(stored.State); err != nil {
		return "", 0, errors.Wrap(err, "read the stored state")
	}
	set, err := state.OperationSet(soID)
	if err != nil {
		return "", 0, err
	}
	nonce, _ := set.AuthorHead(peerID)
	return peerID, nonce, nil
}

// SendSharedObject restarts the account's sync of the Space soID with its
// other members, which sends them the operations they lack.
func (s *LocalTransferSource) SendSharedObject(soID string) bool {
	return s.account.RetrySharedObjectSync(soID)
}

// _ is a type assertion
var (
	_ TransferSource = (*LocalTransferSource)(nil)
	_ CleanupSource  = (*LocalTransferSource)(nil)
)
