package sql_rpc_client

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/tx"
)

// Tx is an ongoing transaction with a Store.
type Tx struct {
	// Ops implements the SQL operations.
	*Ops

	// client is the RPC client for the transaction control stream.
	client sql_rpc.SRPCSql_SqlTransactionClient
	// readOnly indicates if the transaction is read-only.
	readOnly bool
	// released indicates someone already called Commit or Discard.
	released atomic.Bool
}

// InitTx negotiates the transaction with the client stream.
func InitTx(
	ctx context.Context,
	client sql_rpc.SRPCSql_SqlTransactionClient,
	opsCaller rpcstream.RpcStreamCaller[sql_rpc.SRPCSql_SqlTransactionRpcClient],
	write bool,
	dsn string,
) (*Tx, error) {
	err := client.Send(&sql_rpc.SqlTransactionRequest{
		Body: &sql_rpc.SqlTransactionRequest_Init{
			Init: &sql_rpc.SqlTransactionInit{
				Write: write,
				Dsn:   dsn,
			},
		},
	})
	if err != nil {
		_ = client.Close()
		return nil, err
	}

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
		return nil, errors.New("sql rpc: remote returned empty transaction id")
	}

	openStream := rpcstream.NewRpcStreamOpenStream(opsCaller, txID, false)
	openStreamClient := srpc.NewClient(openStream)
	opsClient := sql_rpc.NewSRPCSqlOpsClient(openStreamClient)
	sqlTx := &Tx{
		client:   client,
		readOnly: !write,
	}
	sqlTx.Ops = NewOps(opsClient, &sqlTx.released)
	return sqlTx, nil
}

// Commit commits the transaction to storage.
func (t *Tx) Commit(ctx context.Context) error {
	if t.released.Swap(true) {
		return tx.ErrDiscarded
	}
	err := t.client.Send(&sql_rpc.SqlTransactionRequest{
		Body: &sql_rpc.SqlTransactionRequest_Commit{Commit: true},
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
	if err == nil && !complete.GetCommitted() {
		err = tx.ErrDiscarded
	}
	return mapTxError(err)
}

// Discard cancels the transaction.
func (t *Tx) Discard() {
	if t.released.Swap(true) {
		return
	}
	_ = t.client.Send(&sql_rpc.SqlTransactionRequest{
		Body: &sql_rpc.SqlTransactionRequest_Discard{Discard: true},
	})
	_, _ = t.client.Recv()
	_ = t.client.Close()
}

// GetReadOnly returns if the transaction is read-only.
func (t *Tx) GetReadOnly() bool {
	return t.readOnly
}

// GetSqlOps returns the sql operations interface.
func (t *Tx) GetSqlOps(ctx context.Context) (hydra_sql.SqlOps, error) {
	if t.released.Load() {
		return nil, tx.ErrDiscarded
	}
	return t.Ops, nil
}

// _ is a type assertion.
var _ hydra_sql.SqlTransaction = ((*Tx)(nil))
