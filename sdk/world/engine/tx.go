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
func NewSDKTx(client *resource_client.Client, ref resource_client.ResourceRef, readOnly bool) (*SDKTx, error) {
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
// Releasing the resource reference triggers server-side cleanup which
// includes discarding the underlying transaction.
func (tx *SDKTx) Discard() {
	tx.ref.Release()
}

// _ is a type assertion
var _ world.Tx = (*SDKTx)(nil)
