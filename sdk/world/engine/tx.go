package sdk_world_engine

import (
	"context"
	"sync/atomic"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKTx implements world.Tx over SRPC by delegating to
// TxResourceService and WorldStateResourceService calls.
type SDKTx struct {
	*SDKWorldState
	txService s4wave_world.SRPCTxResourceServiceClient
	finished  atomic.Bool // a terminal RPC has returned; only reference release remains
	hold      *writeHold  // opener of a write transaction; nil for a read
}

// NewSDKTx creates a new SDKTx wrapping a resource reference.
// The reference must point to a TxResource on the server.
func NewSDKTx(client ResourceClient, ref resource_client.ResourceRef, readOnly bool) (*SDKTx, error) {
	// Wrap the transaction resource as a World state with its read-only setting.
	ws, err := NewSDKWorldState(client, ref, readOnly)
	if err != nil {
		return nil, err
	}

	// Acquire the transaction client for commit and discard requests.
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}

	return &SDKTx{
		SDKWorldState: ws,
		txService:     s4wave_world.NewSRPCTxResourceServiceClient(srpcClient),
	}, nil
}

// Commit commits the transaction to storage. If ctx is already done, Commit
// returns ctx's error and writes nothing. The request ignores ctx's
// cancellation from the call on, so a caller's deadline does not abandon a
// commit that started. Closing the transport still ends the call. A commit
// rejected because another writer advanced the World returns an error
// world.IsSuperseded matches, and one whose tree references a block the Volume
// does not hold returns an error matching block.ErrRootNotHeld.
func (tx *SDKTx) Commit(ctx context.Context) error {
	// Refuse a caller that has already stopped waiting, then release the resource.
	if err := ctx.Err(); err != nil {
		tx.finished.Store(true)
		tx.ref.Release()
		tx.hold.finish("canceled")
		return err
	}

	// Send the commit, then release the finished transaction resource.
	err := s4wave_world.CommitRemote(ctx, tx.txService)
	tx.finished.Store(true)
	tx.ref.Release()
	tx.hold.finish("commit")
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
func (tx *SDKTx) Discard() {
	if !tx.finished.Load() {
		_, _ = tx.txService.Discard(context.Background(), &s4wave_world.DiscardRequest{})
		tx.finished.Store(true)
		tx.hold.finish("discard")
	}
	tx.ref.Release()
}

var _ world.Tx = (*SDKTx)(nil)
