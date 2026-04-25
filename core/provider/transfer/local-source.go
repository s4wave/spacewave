package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/core/bstore"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
)

// LocalTransferSource implements TransferSource for a local provider account.
type LocalTransferSource struct {
	account    *provider_local.ProviderAccount
	providerID string
	accountID  string
	b          bus.Bus
}

// NewLocalTransferSource creates a new LocalTransferSource.
func NewLocalTransferSource(
	account *provider_local.ProviderAccount,
	providerID, accountID string,
	b bus.Bus,
) *LocalTransferSource {
	return &LocalTransferSource{
		account:    account,
		providerID: providerID,
		accountID:  accountID,
		b:          b,
	}
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

// GetSharedObjectState reads the SO state for a shared object from the object store.
func (s *LocalTransferSource) GetSharedObjectState(ctx context.Context, sharedObjectID string) (*sobject.SOState, error) {
	objStore, rel, err := s.buildObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	key := provider_local.SobjectObjectStoreHostStateKey(sharedObjectID)
	data, found, err := otx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, sobject.ErrSharedObjectNotFound
	}

	state := &sobject.SOState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return state, nil
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
// Uses the GC ref graph to enumerate blocks belonging to the bucket.
func (s *LocalTransferSource) GetBlockRefs(ctx context.Context, ref *sobject.SharedObjectRef) ([]*block.BlockRef, error) {
	vol := s.account.GetVolume()
	rg := vol.GetRefGraph()
	if rg == nil {
		return nil, nil
	}

	blockStoreID := ref.GetBlockStoreId()
	bucketID := provider_local.BlockStoreBucketID(s.providerID, s.accountID, blockStoreID)
	bucketIRI := block_gc.BucketIRI(bucketID)

	outgoing, err := rg.GetOutgoingRefs(ctx, bucketIRI)
	if err != nil {
		return nil, err
	}

	var refs []*block.BlockRef
	for _, iri := range outgoing {
		br, ok := block_gc.ParseBlockIRI(iri)
		if ok {
			refs = append(refs, br)
		}
	}
	return refs, nil
}

// buildObjectStore builds an object store handle for the source account.
func (s *LocalTransferSource) buildObjectStore(ctx context.Context) (object.ObjectStore, func(), error) {
	objStoreID := provider_local.SobjectObjectStoreID(s.providerID, s.accountID)
	volID := s.account.GetVolume().GetID()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, s.b, false, objStoreID, volID, nil)
	if err != nil {
		return nil, nil, err
	}
	return handle.GetObjectStore(), diRef.Release, nil
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
