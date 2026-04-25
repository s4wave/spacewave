package provider_transfer

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// TransferSource provides read access to a provider account for transfer.
type TransferSource interface {
	// GetSharedObjectList returns the list of shared objects on the source.
	GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error)
	// GetSharedObjectState reads the SO state for a shared object.
	GetSharedObjectState(ctx context.Context, sharedObjectID string) (*sobject.SOState, error)
	// GetBlockStore returns the block store ops for a shared object's block store.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
	// GetBlockRefs returns all block refs tracked for a shared object's block store.
	// Uses the GC ref graph to enumerate blocks belonging to the bucket.
	GetBlockRefs(ctx context.Context, ref *sobject.SharedObjectRef) ([]*block.BlockRef, error)
}

// CleanupSource handles post-merge cleanup of the source account.
// This is separate from TransferSource to keep the read interface minimal.
type CleanupSource interface {
	// DeleteSharedObject deletes a shared object from the source account.
	DeleteSharedObject(ctx context.Context, soID string) error
	// DeleteVolume deletes the source account's storage volume.
	DeleteVolume(ctx context.Context) error
}
