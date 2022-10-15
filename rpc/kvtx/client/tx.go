package rpc_kvtx_client

import (
	"context"
	"errors"
	"sync/atomic"

	rpc_kvtx "github.com/aperturerobotics/bldr/rpc/kvtx"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
)

// Tx is an ongoing transaction with a Store.
type Tx struct {
	// Ops implements the TxOps.
	*Ops

	// client is the RPC client for the transaction control stream.
	client rpc_kvtx.SRPCKvtx_KvtxTransactionClient
	// released indicates someone already called Commit or Discard.
	released atomic.Bool
}

// InitTx negotiates the transaction with the client stream.
// le can be nil to disable error logging
// note: usually you will want to call Store.NewTransaction()
func InitTx(
	ctx context.Context,
	client rpc_kvtx.SRPCKvtx_KvtxTransactionClient,
	opsCaller rpcstream.RpcStreamCaller[rpc_kvtx.SRPCKvtx_KvtxTransactionRpcClient],
) (*Tx, error) {
	resp, err := client.Recv()
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	ackMsg := resp.GetAck()
	if errStr := ackMsg.GetError(); errStr != "" {
		_ = client.Close()
		return nil, errors.New(errStr)
	}

	txID := ackMsg.GetTransactionId()
	if txID == "" {
		_ = client.Close()
		return nil, errors.New("rpc_kvtx: remote returned empty transaction id")
	}

	openStream := rpcstream.NewRpcStreamOpenStream(opsCaller, txID)
	openStreamClient := srpc.NewClient(openStream)
	opsClient := rpc_kvtx.NewSRPCKvtxOpsClient(openStreamClient)
	return &Tx{
		Ops:    NewOps(ctx, opsClient),
		client: client,
	}, nil
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) error {
	if t.released.Swap(true) {
		return kvtx.ErrDiscarded
	}
	err := t.client.Send(&rpc_kvtx.KvtxTransactionRequest{
		Body: &rpc_kvtx.KvtxTransactionRequest_Commit{Commit: true},
	})
	if err != nil {
		_ = t.client.Close()
		return err
	}
	resp, err := t.client.Recv()
	if err != nil {
		_ = t.client.Close()
		return err
	}
	complete := resp.GetComplete()
	if errStr := complete.GetError(); errStr != "" {
		err = errors.New(errStr)
	}
	if err == nil && !resp.GetComplete().GetCommitted() {
		err = kvtx.ErrDiscarded
	}
	return err
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	if t.released.Swap(true) {
		return
	}
	_ = t.client.Send(&rpc_kvtx.KvtxTransactionRequest{
		Body: &rpc_kvtx.KvtxTransactionRequest_Discard{Discard: true},
	})
	// wait for the remote to ack the Discard
	// resp.GetComplete().GetDiscarded() == true
	_, _ = t.client.Recv()
	_ = t.client.Close()
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
