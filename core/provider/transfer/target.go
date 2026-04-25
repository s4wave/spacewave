package provider_transfer

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// TransferTarget provides write access to a provider account for transfer.
type TransferTarget interface {
	// GetBlockStore returns the block store ops for a shared object's block store.
	// Creates the block store if it does not exist.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
	// AddSharedObject adds a shared object to the target's SO list.
	// The ref should already have the target's provider resource ref.
	AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta) error
	// WriteSharedObjectState writes the SO state for a shared object.
	WriteSharedObjectState(ctx context.Context, sharedObjectID string, state *sobject.SOState) error
}
