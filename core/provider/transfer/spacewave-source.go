package provider_transfer

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// SpacewaveTransferSource implements TransferSource for a spacewave cloud account.
type SpacewaveTransferSource struct {
	account    *provider_spacewave.ProviderAccount
	providerID string
	accountID  string
}

// NewSpacewaveTransferSource creates a new SpacewaveTransferSource.
func NewSpacewaveTransferSource(
	account *provider_spacewave.ProviderAccount,
	providerID, accountID string,
) *SpacewaveTransferSource {
	return &SpacewaveTransferSource{
		account:    account,
		providerID: providerID,
		accountID:  accountID,
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

	list := &sobject.SharedObjectList{}
	if err := list.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal shared object list")
	}
	return list, nil
}

// GetSharedObjectState reads the SO state for a shared object from the cloud.
func (s *SpacewaveTransferSource) GetSharedObjectState(ctx context.Context, sharedObjectID string) (*sobject.SOState, error) {
	cli := s.account.GetSessionClient()
	data, err := cli.GetSOState(ctx, sharedObjectID, 0, provider_spacewave.SeedReasonColdSeed)
	if err != nil {
		return nil, errors.Wrap(err, "get SO state from cloud")
	}
	if len(data) == 0 {
		return nil, sobject.ErrSharedObjectNotFound
	}

	state := &sobject.SOState{}
	if err := state.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal SO state")
	}
	return state, nil
}

// GetBlockStore returns the block store ops for reading blocks from a shared object.
func (s *SpacewaveTransferSource) GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error) {
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
