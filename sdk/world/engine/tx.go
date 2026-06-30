package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKTx implements world.Tx over SRPC by delegating to
// TxResourceService and WorldStateResourceService calls.
type SDKTx struct {
	*SDKWorldState
	txService s4wave_world.SRPCTxResourceServiceClient
}

// NewSDKTx creates a new SDKTx wrapping a resource reference.
// The reference must point to a TxResource on the server.
func NewSDKTx(client ResourceClient, ref resource_client.ResourceRef, readOnly bool) (*SDKTx, error) {
	ws, err := NewSDKWorldState(client, ref, readOnly)
	if err != nil {
		return nil, err
	}

	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}

	return &SDKTx{
		SDKWorldState: ws,
		txService:     s4wave_world.NewSRPCTxResourceServiceClient(srpcClient),
	}, nil
}

// Commit commits the transaction to storage.
func (tx *SDKTx) Commit(ctx context.Context) error {
	_, err := tx.txService.Commit(ctx, &s4wave_world.CommitRequest{})
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
func (tx *SDKTx) Discard() {
	_, _ = tx.txService.Discard(context.Background(), &s4wave_world.DiscardRequest{})
	tx.ref.Release()
}

var _ world.Tx = (*SDKTx)(nil)
