//go:build js

// Package sqlite_wasm implements a database/sql driver for sqlite.wasm
// via starpc RPC to a dedicated Worker running sqlite.wasm with OPFS
// persistence. Registers as "sqlite3-wasm" with database/sql.
package sqlite_wasm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"

	"github.com/s4wave/spacewave/db/sql/sqlite-wasm/rpc"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// driverName is the name registered with database/sql.
const driverName = "sqlite3-wasm"

// clientMu guards the global RPC client reference.
var clientMu sync.Mutex

// clientCh is closed when a client becomes available.
var clientCh = make(chan struct{})

// globalClient is the current RPC client, set by SetClient.
var globalClient sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient

func init() {
	sql.Register(driverName, &wasmDriver{})
}

// SetClient sets the RPC client used by the driver.
// Call with nil to clear the client (e.g. on Worker disconnect).
func SetClient(client sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient) {
	clientMu.Lock()
	globalClient = client
	if client != nil {
		// Signal waiters.
		select {
		case <-clientCh:
			// Already closed, make a new channel for future waits.
		default:
			close(clientCh)
		}
	} else {
		// Reset the channel so future waiters block.
		clientCh = make(chan struct{})
	}
	clientMu.Unlock()
}

// getClient returns the current RPC client, blocking until one is available.
func getClient(ctx context.Context) (sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient, error) {
	clientMu.Lock()
	c := globalClient
	ch := clientCh
	clientMu.Unlock()
	if c != nil {
		return c, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ch:
		clientMu.Lock()
		c = globalClient
		clientMu.Unlock()
		if c == nil {
			return nil, errors.New("sqlite-wasm: client was cleared")
		}
		return c, nil
	}
}

// wasmDriver implements database/sql/driver.Driver.
type wasmDriver struct{}

// Open opens a new connection to the database.
// The name is the database path (e.g. "/vol-123.db").
func (d *wasmDriver) Open(name string) (driver.Conn, error) {
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "sqlite-wasm: get client")
	}
	resp, err := client.OpenDb(ctx, &sql_sqlite_wasm_rpc.OpenDbRequest{Path: name})
	if err != nil {
		return nil, errors.Wrap(err, "sqlite-wasm: open db")
	}
	return &wasmConn{client: client, dbID: resp.GetDbId()}, nil
}

// wasmConn implements database/sql/driver.Conn.
type wasmConn struct {
	client sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient
	dbID   uint32
}

// Prepare returns a prepared statement.
func (c *wasmConn) Prepare(query string) (driver.Stmt, error) {
	return &wasmStmt{conn: c, query: query}, nil
}

// Close closes the database connection.
func (c *wasmConn) Close() error {
	_, err := c.client.CloseDb(context.Background(), &sql_sqlite_wasm_rpc.CloseDbRequest{DbId: c.dbID})
	return err
}

// Begin starts a transaction.
func (c *wasmConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts a transaction with optional read-only semantics.
func (c *wasmConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) {
		return nil, errors.New("sqlite-wasm: unsupported isolation level")
	}

	beginSQL := "BEGIN IMMEDIATE"
	if opts.ReadOnly {
		beginSQL = "BEGIN"
	}

	_, err := c.client.Exec(ctx, &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: c.dbID,
		Sql:  beginSQL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "sqlite-wasm: begin")
	}
	return &wasmTx{conn: c}, nil
}

// ExecContext executes a statement that does not return rows.
func (c *wasmConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	params := namedValuesToProto(args)
	resp, err := c.client.Exec(ctx, &sql_sqlite_wasm_rpc.ExecRequest{
		DbId:   c.dbID,
		Sql:    query,
		Params: params,
	})
	if err != nil {
		return nil, err
	}
	return &wasmResult{
		changes:      resp.GetChanges(),
		lastInsertID: resp.GetLastInsertRowId(),
	}, nil
}

// QueryContext executes a query that returns rows.
func (c *wasmConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	params := namedValuesToProto(args)
	stream, err := c.client.Query(ctx, &sql_sqlite_wasm_rpc.QueryRequest{
		DbId:   c.dbID,
		Sql:    query,
		Params: params,
	})
	if err != nil {
		return nil, err
	}
	// Read the first message to get column names.
	first, err := stream.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "sqlite-wasm: query recv columns")
	}
	return &wasmRows{stream: stream, cols: first.GetColumnNames()}, nil
}

// wasmTx implements database/sql/driver.Tx.
type wasmTx struct {
	conn *wasmConn
}

// Commit commits the transaction.
func (t *wasmTx) Commit() error {
	_, err := t.conn.client.Exec(context.Background(), &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: t.conn.dbID,
		Sql:  "COMMIT",
	})
	return err
}

// Rollback rolls back the transaction.
func (t *wasmTx) Rollback() error {
	_, err := t.conn.client.Exec(context.Background(), &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: t.conn.dbID,
		Sql:  "ROLLBACK",
	})
	return err
}

// wasmStmt implements database/sql/driver.Stmt.
type wasmStmt struct {
	conn  *wasmConn
	query string
}

// Close is a no-op (statements are not prepared on the server).
func (s *wasmStmt) Close() error { return nil }

// NumInput returns -1 (unknown number of inputs).
func (s *wasmStmt) NumInput() int { return -1 }

// Exec executes the statement.
func (s *wasmStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := valuesToNamed(args)
	return s.conn.ExecContext(context.Background(), s.query, named)
}

// Query executes the statement as a query.
func (s *wasmStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := valuesToNamed(args)
	return s.conn.QueryContext(context.Background(), s.query, named)
}

// wasmResult implements database/sql/driver.Result.
type wasmResult struct {
	changes      int64
	lastInsertID int64
}

// LastInsertId returns the last insert row ID.
func (r *wasmResult) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

// RowsAffected returns the number of rows affected.
func (r *wasmResult) RowsAffected() (int64, error) {
	return r.changes, nil
}

// wasmRows implements database/sql/driver.Rows backed by a streaming RPC.
type wasmRows struct {
	stream sql_sqlite_wasm_rpc.SRPCSqliteBridge_QueryClient
	cols   []string
}

// Columns returns the column names.
func (r *wasmRows) Columns() []string {
	return r.cols
}

// Close closes the rows iterator.
func (r *wasmRows) Close() error {
	err := r.stream.Close()
	if errors.Is(err, srpc.ErrCompleted) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// Next populates dest with the values of the next row.
func (r *wasmRows) Next(dest []driver.Value) error {
	msg, err := r.stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		return err
	}
	row := msg.GetRow()
	for i := range dest {
		if i < len(row) {
			dest[i] = protoToDriverValue(row[i])
		}
	}
	return nil
}

// namedValuesToProto converts driver.NamedValue args to proto SqlValue params.
func namedValuesToProto(args []driver.NamedValue) []*sql_sqlite_wasm_rpc.SqlValue {
	if len(args) == 0 {
		return nil
	}
	params := make([]*sql_sqlite_wasm_rpc.SqlValue, len(args))
	for i, arg := range args {
		params[i] = goToProtoValue(arg.Value)
	}
	return params
}

// valuesToNamed converts driver.Value slice to driver.NamedValue slice.
func valuesToNamed(args []driver.Value) []driver.NamedValue {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return named
}

// goToProtoValue converts a Go driver value to a proto SqlValue.
func goToProtoValue(v driver.Value) *sql_sqlite_wasm_rpc.SqlValue {
	if v == nil {
		return &sql_sqlite_wasm_rpc.SqlValue{}
	}
	switch val := v.(type) {
	case int64:
		return &sql_sqlite_wasm_rpc.SqlValue{
			Value: &sql_sqlite_wasm_rpc.SqlValue_IntValue{IntValue: val},
		}
	case float64:
		return &sql_sqlite_wasm_rpc.SqlValue{
			Value: &sql_sqlite_wasm_rpc.SqlValue_FloatValue{FloatValue: val},
		}
	case string:
		return &sql_sqlite_wasm_rpc.SqlValue{
			Value: &sql_sqlite_wasm_rpc.SqlValue_StrValue{StrValue: val},
		}
	case []byte:
		return &sql_sqlite_wasm_rpc.SqlValue{
			Value: &sql_sqlite_wasm_rpc.SqlValue_BlobValue{BlobValue: val},
		}
	default:
		return &sql_sqlite_wasm_rpc.SqlValue{}
	}
}

// protoToDriverValue converts a proto SqlValue to a Go driver.Value.
func protoToDriverValue(v *sql_sqlite_wasm_rpc.SqlValue) driver.Value {
	if v == nil {
		return nil
	}
	switch val := v.GetValue().(type) {
	case *sql_sqlite_wasm_rpc.SqlValue_IntValue:
		return val.IntValue
	case *sql_sqlite_wasm_rpc.SqlValue_FloatValue:
		return val.FloatValue
	case *sql_sqlite_wasm_rpc.SqlValue_StrValue:
		return val.StrValue
	case *sql_sqlite_wasm_rpc.SqlValue_BlobValue:
		return val.BlobValue
	default:
		return nil
	}
}

// DeleteDatabase deletes a database by path via the RPC client.
func DeleteDatabase(path string) error {
	client, err := getClient(context.Background())
	if err != nil {
		return errors.Wrap(err, "sqlite-wasm: get client for delete")
	}
	_, err = client.DeleteDb(context.Background(), &sql_sqlite_wasm_rpc.DeleteDbRequest{Path: path})
	return err
}

// _ is a type assertion.
var _ driver.Driver = (*wasmDriver)(nil)

// _ is a type assertion.
var _ driver.ExecerContext = (*wasmConn)(nil)

// _ is a type assertion.
var _ driver.ConnBeginTx = (*wasmConn)(nil)

// _ is a type assertion.
var _ driver.QueryerContext = (*wasmConn)(nil)
