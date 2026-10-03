package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
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

// GetSharedObjectList returns the list of shared objects on the source account.
func (s *LocalTransferSource) GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	ctr, rel, err := s.account.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rel()

	val, err := ctr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return val.CloneVT(), nil
}

// ReplaySharedObject returns the World of a Space after its last operation.
func (s *LocalTransferSource) ReplaySharedObject(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) (*sobject_world_engine.InnerState, error) {
	return replaySharedObject(ctx, le, s.b, s.account.GetStepFactorySet(), s.account, ref)
}

// GetBlockStore returns the block store ops for reading blocks from a shared object.
func (s *LocalTransferSource) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
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

// GetBlockRefs returns all block refs tracked for a shared object's block store.
func (s *LocalTransferSource) GetBlockRefs(ctx context.Context, ref *sobject.SharedObjectRef) ([]*block.BlockRef, error) {
	return s.account.ListBlockStoreRefs(ctx, ref.GetBlockStoreId())
}

// DeleteSharedObject deletes a shared object from the source account.
func (s *LocalTransferSource) DeleteSharedObject(ctx context.Context, soID string) error {
	return s.account.DeleteSharedObject(ctx, soID)
}

// DeleteVolume deletes the source account's storage volume.
func (s *LocalTransferSource) DeleteVolume(ctx context.Context) error {
	return s.account.GetVolume().Delete()
}

// _ is a type assertion
var (
	_ TransferSource = (*LocalTransferSource)(nil)
	_ CleanupSource  = (*LocalTransferSource)(nil)
)
