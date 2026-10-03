// Package sqlite_rpc connects database/sql to an instance-bound SQLite bridge.
package sqlite_rpc

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_sqlite_wasm_rpc "github.com/s4wave/spacewave/db/sql/sqlite-wasm/rpc"
)

// connection implements database/sql/driver.Conn.
type connection struct {
	// client owns the transport to the physical SQLite connections.
	client sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient
	// dbID identifies this connection within the bridge.
	dbID uint32
}

// Prepare returns a prepared statement.
func (c *connection) Prepare(query string) (driver.Stmt, error) {
	return &statement{conn: c, query: query}, nil
}

// Close closes the database connection.
func (c *connection) Close() error {
	_, err := c.client.CloseDb(context.Background(), &sql_sqlite_wasm_rpc.CloseDbRequest{DbId: c.dbID})
	return err
}

// Begin starts a transaction.
func (c *connection) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts a transaction with optional read-only semantics.
func (c *connection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	// Require the isolation level supported by the SQLite bridge.
	if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) {
		return nil, errors.New("sqlite-rpc: unsupported isolation level")
	}

	// Choose the SQLite transaction mode from the read-only option.
	beginSQL := "BEGIN IMMEDIATE"
	if opts.ReadOnly {
		beginSQL = "BEGIN"
	}

	// Begin the transaction on the bound SQLite connection.
	_, err := c.client.Exec(ctx, &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: c.dbID,
		Sql:  beginSQL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "sqlite-rpc: begin")
	}
	return &transaction{conn: c}, nil
}

// ExecContext executes a statement that does not return rows.
func (c *connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	// Encode the SQL arguments for the SQLite bridge.
	params, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}

	// Execute the SQL statement on the bound SQLite connection.
	resp, err := c.client.Exec(ctx, &sql_sqlite_wasm_rpc.ExecRequest{
		DbId:   c.dbID,
		Sql:    query,
		Params: params,
	})
	if err != nil {
		return nil, err
	}
	return &result{
		changes:      resp.GetChanges(),
		lastInsertID: resp.GetLastInsertRowId(),
	}, nil
}

// QueryContext executes a query that returns rows.
func (c *connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// Encode the SQL arguments for the SQLite bridge.
	params, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}

	// Open the query stream on the bound SQLite connection.
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
		_ = stream.Close()
		return nil, errors.Wrap(err, "sqlite-rpc: query recv columns")
	}
	return &rows{stream: stream, cols: first.GetColumnNames()}, nil
}

// transaction implements database/sql/driver.Tx.
type transaction struct {
	// conn retains the physical connection through commit or rollback.
	conn *connection
}

// Commit commits the transaction.
func (t *transaction) Commit() error {
	_, err := t.conn.client.Exec(context.Background(), &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: t.conn.dbID,
		Sql:  "COMMIT",
	})
	return err
}

// Rollback rolls back the transaction.
func (t *transaction) Rollback() error {
	_, err := t.conn.client.Exec(context.Background(), &sql_sqlite_wasm_rpc.ExecRequest{
		DbId: t.conn.dbID,
		Sql:  "ROLLBACK",
	})
	return err
}

// statement implements database/sql/driver.Stmt.
type statement struct {
	// conn owns statement execution.
	conn *connection
	// query is prepared by the bridge when executed.
	query string
}

// Close is a no-op (statements are not prepared on the server).
func (s *statement) Close() error { return nil }

// NumInput returns -1 (unknown number of inputs).
func (s *statement) NumInput() int { return -1 }

// Exec executes the statement.
func (s *statement) Exec(args []driver.Value) (driver.Result, error) {
	named := valuesToNamed(args)
	return s.conn.ExecContext(context.Background(), s.query, named)
}

// Query executes the statement as a query.
func (s *statement) Query(args []driver.Value) (driver.Rows, error) {
	named := valuesToNamed(args)
	return s.conn.QueryContext(context.Background(), s.query, named)
}

// result implements database/sql/driver.Result.
type result struct {
	// changes is the number of affected rows.
	changes int64
	// lastInsertID is the SQLite row ID from the completed statement.
	lastInsertID int64
}

// LastInsertId returns the last insert row ID.
func (r *result) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

// RowsAffected returns the number of rows affected.
func (r *result) RowsAffected() (int64, error) {
	return r.changes, nil
}

// rows implements database/sql/driver.Rows backed by a streaming RPC.
type rows struct {
	// stream owns the query cursor until Close.
	stream sql_sqlite_wasm_rpc.SRPCSqliteBridge_QueryClient
	// cols is the ordered column header from the bridge.
	cols []string
}

// Columns returns the column names.
func (r *rows) Columns() []string {
	return r.cols
}

// Close closes the rows iterator.
func (r *rows) Close() error {
	err := r.stream.Close()
	if errors.Is(err, srpc.ErrCompleted) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// Next populates dest with the values of the next row.
func (r *rows) Next(dest []driver.Value) error {
	// Receive the next SQLite row and propagate stream completion.
	msg, err := r.stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		return err
	}

	// Require the SQLite row to match the driver column count.
	row := msg.GetRow()
	if len(row) != len(dest) {
		return fmt.Errorf("sqlite-rpc: received %d values for %d columns", len(row), len(dest))
	}

	// Decode the SQLite row values into the driver destination.
	for i := range dest {
		dest[i] = protoToDriverValue(row[i])
	}
	return nil
}

// namedValuesToProto converts driver.NamedValue args to proto SqlValue params.
func namedValuesToProto(args []driver.NamedValue) ([]*hydra_sql.SqlValue, error) {
	if len(args) == 0 {
		return nil, nil
	}
	params := make([]*hydra_sql.SqlValue, len(args))
	for i, arg := range args {
		if arg.Name != "" {
			return nil, errors.New("sqlite-rpc: named parameters are unsupported")
		}
		value, err := goToProtoValue(arg.Value)
		if err != nil {
			return nil, err
		}
		params[i] = value
	}
	return params, nil
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
func goToProtoValue(v driver.Value) (*hydra_sql.SqlValue, error) {
	if v == nil {
		return &hydra_sql.SqlValue{}, nil
	}
	switch val := v.(type) {
	case int64:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_IntValue{IntValue: val},
		}, nil
	case float64:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_FloatValue{FloatValue: val},
		}, nil
	case string:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_StrValue{StrValue: val},
		}, nil
	case []byte:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_BlobValue{BlobValue: bytes.Clone(val)},
		}, nil
	case bool:
		var integer int64
		if val {
			integer = 1
		}
		return &hydra_sql.SqlValue{Value: &hydra_sql.SqlValue_IntValue{IntValue: integer}}, nil
	default:
		return nil, fmt.Errorf("sqlite-rpc: unsupported parameter type %T", v)
	}
}

// protoToDriverValue converts a proto SqlValue to a Go driver.Value.
func protoToDriverValue(v *hydra_sql.SqlValue) driver.Value {
	if v == nil {
		return nil
	}
	switch val := v.GetValue().(type) {
	case *hydra_sql.SqlValue_IntValue:
		return val.IntValue
	case *hydra_sql.SqlValue_FloatValue:
		return val.FloatValue
	case *hydra_sql.SqlValue_StrValue:
		return val.StrValue
	case *hydra_sql.SqlValue_BlobValue:
		return bytes.Clone(val.BlobValue)
	default:
		return nil
	}
}

// _ is a type assertion.
var (
	_ driver.Conn           = (*connection)(nil)
	_ driver.ConnBeginTx    = (*connection)(nil)
	_ driver.ExecerContext  = (*connection)(nil)
	_ driver.QueryerContext = (*connection)(nil)
)
