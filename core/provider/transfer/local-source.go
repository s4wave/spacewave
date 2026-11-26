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
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// LocalTransferSource implements TransferSource for a local provider account.
type LocalTransferSource struct {
	// account is the provider account behind this endpoint.
	account *provider_local.ProviderAccount
	// b mounts transfer stores on the endpoint bus.
	b bus.Bus
}

// NewLocalTransferSource creates a new LocalTransferSource.
func NewLocalTransferSource(
	account *provider_local.ProviderAccount,
	b bus.Bus,
) *LocalTransferSource {
	return &LocalTransferSource{account: account, b: b}
}

// GetVolume returns the Volume whose identity this endpoint transfers.
func (s *LocalTransferSource) GetVolume() volume.Volume {
	return s.account.GetVolume()
}

// ReadLinkedCloudAccountID reads a session's linked-cloud account from this source.
// It returns an empty string when the session has no cloud link.
// accountID is the account in the session reference.
// The mounted Volume ID can differ from the underlying storage Volume ID.
func (s *LocalTransferSource) ReadLinkedCloudAccountID(ctx context.Context, accountID, sessionID string) (string, error) {
	// Locate the local session object store.
	objectStoreID := provider_local.SessionObjectStoreID(provider_local.ProviderID, accountID)
	volID := s.GetVolume().GetID()

	// Open that object store.
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, s.b, false, objectStoreID, volID, nil)
	if err != nil {
		return "", errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	// Open a read transaction on the object store.
	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "new read transaction")
	}
	defer otx.Discard()

	// Return the linked-cloud account ID, or empty when none is stored.
	key := provider_local.LinkedCloudKey(sessionID)
	data, found, err := otx.Get(ctx, key)
	if err != nil {
		return "", errors.Wrap(err, "read linked-cloud key")
	}
	if !found {
		return "", nil
	}
	return string(data), nil
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

// DeleteVolume deletes the source account's storage volume.
func (s *LocalTransferSource) DeleteVolume(ctx context.Context) error {
	return s.account.GetVolume().Delete()
}

// _ verifies the endpoint contracts.
var (
	_ TransferSource = (*LocalTransferSource)(nil)
	_ CleanupSource  = (*LocalTransferSource)(nil)
)
