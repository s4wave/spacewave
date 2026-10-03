package provider_transfer

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// TransferTarget provides write access to a provider account for transfer.
type TransferTarget interface {
	// GetBlockStore returns the block store ops for a shared object's block store.
	// Creates the block store if it does not exist.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
	// AddSharedObject adds a shared object to the target's SO list.
	// The ref should already have the target's provider resource ref.
	AddSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta) error
	// WriteSharedObjectState replaces the state of a shared object with a
	// new lineage owned solely by owner whose checkpoint holds stateData.
	WriteSharedObjectState(ctx context.Context, le *logrus.Entry, sharedObjectID string, owner crypto.PrivKey, stateData []byte) error
}
