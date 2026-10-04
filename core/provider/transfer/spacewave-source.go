package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// SpacewaveTransferSource implements TransferSource for a spacewave cloud account.
type SpacewaveTransferSource struct {
	account    *provider_spacewave.ProviderAccount
	providerID string
	accountID  string
	b          bus.Bus
}

// NewSpacewaveTransferSource creates a new SpacewaveTransferSource.
func NewSpacewaveTransferSource(
	account *provider_spacewave.ProviderAccount,
	providerID, accountID string,
	b bus.Bus,
) *SpacewaveTransferSource {
	return &SpacewaveTransferSource{
		account:    account,
		providerID: providerID,
		accountID:  accountID,
		b:          b,
	}
}

// GetAccount returns the underlying spacewave provider account.
func (s *SpacewaveTransferSource) GetAccount() *provider_spacewave.ProviderAccount {
	return s.account
}

// GetSharedObjectList returns the list of shared objects from the cloud.
func (s *SpacewaveTransferSource) GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	cli := s.account.GetSessionClient()
	data, err := cli.ListSharedObjects(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list shared objects from cloud")
	}

	return provider_spacewave.DecodeSharedObjectList(data, s.providerID)
}

// ReplaySharedObject returns the World of a Space after its last operation.
func (s *SpacewaveTransferSource) ReplaySharedObject(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) (*sobject_world_engine.InnerState, error) {
	return replaySharedObject(ctx, le, s.b, s.account.GetStepFactorySet(), s.account, ref)
}

// GetBlockStore returns the block store ops for reading blocks from a shared object.
func (s *SpacewaveTransferSource) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
	// Mount the source block store and return its release function.
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
// Enumerates blocks by pulling the packfile manifest from the cloud and scanning
// each packfile's index entries.
func (s *SpacewaveTransferSource) GetBlockRefs(ctx context.Context, ref *sobject.SharedObjectRef) ([]*block.BlockRef, error) {
	bstoreID := ref.GetBlockStoreId()
	return s.account.EnumerateBlockRefs(ctx, bstoreID)
}

// _ is a type assertion
var _ TransferSource = (*SpacewaveTransferSource)(nil)
