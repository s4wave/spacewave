package sql_rpc_client

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	"github.com/s4wave/spacewave/db/tx"
)

// Ops implements SqlOps with a SqlOps service.
type Ops struct {
	// client is the service client.
	client sql_rpc.SRPCSqlOpsClient
	// released indicates if the transaction has been released.
	released *atomic.Bool
}

// NewOps constructs a new SQL ops client.
func NewOps(client sql_rpc.SRPCSqlOpsClient, released *atomic.Bool) *Ops {
	if released == nil {
		released = &atomic.Bool{}
	}
	return &Ops{
		client:   client,
		released: released,
	}
}

// Exec executes a query that doesn't return rows.
func (o *Ops) Exec(query string, args []driver.Value) (driver.Result, error) {
	return o.ExecContext(context.Background(), query, sql_rpc.ValuesToNamedValues(args))
}

// ExecContext executes a query that doesn't return rows.
func (o *Ops) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if o.released.Load() {
		return nil, tx.ErrDiscarded
	}
	wireArgs, err := sql_rpc.NamedValuesToSqlValues(args)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Exec(ctx, &sql_rpc.SqlExecRequest{
		Query: query,
		Args:  wireArgs,
	})
	if err != nil {
		return nil, mapTxError(err)
	}
	if err := rpcErr(nil, resp.GetError()); err != nil {
		return nil, err
	}
	return &result{
		lastInsertID: resp.GetLastInsertId(),
		rowsAffected: resp.GetRowsAffected(),
	}, nil
}

// Query executes a query that may return rows.
func (o *Ops) Query(query string, args []driver.Value) (driver.Rows, error) {
	return o.QueryContext(context.Background(), query, sql_rpc.ValuesToNamedValues(args))
}

// QueryContext executes a query that may return rows.
func (o *Ops) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if o.released.Load() {
		return nil, tx.ErrDiscarded
	}
	wireArgs, err := sql_rpc.NamedValuesToSqlValues(args)
	if err != nil {
		return nil, err
	}
	client, err := o.client.Query(ctx)
	if err != nil {
		return nil, mapTxError(err)
	}
	if err := client.Send(&sql_rpc.SqlQueryRequest{
		Body: &sql_rpc.SqlQueryRequest_Init{
			Init: &sql_rpc.SqlQueryInit{
				Query: query,
				Args:  wireArgs,
			},
		},
	}); err != nil {
		_ = client.Close()
		return nil, mapTxError(err)
	}
	resp, err := client.Recv()
	if err != nil {
		_ = client.Close()
		return nil, mapTxError(err)
	}
	if errStr := resp.GetReqError(); errStr != "" {
		_ = client.Close()
		return nil, errors.New(errStr)
	}
	ack := resp.GetAck()
	if ack == nil {
		_ = client.Close()
		return nil, errors.New("sql rpc: expected query ack")
	}
	return newRows(client, ack.GetColumns()), nil
}

func rpcErr(err error, errStr string) error {
	if err == nil && errStr == "" {
		return nil
	}
	if err == nil {
		err = errors.New(errStr)
	}
	return mapTxError(err)
}

func mapTxError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	switch errStr {
	case srpc.ErrCompleted.Error():
		return tx.ErrDiscarded
	case io.EOF.Error():
		return tx.ErrDiscarded
	case context.Canceled.Error():
		return tx.ErrDiscarded
	case tx.ErrDiscarded.Error():
		return tx.ErrDiscarded
	}
	return err
}

// _ is a type assertion.
var _ hydra_sql.SqlOps = (*Ops)(nil)
