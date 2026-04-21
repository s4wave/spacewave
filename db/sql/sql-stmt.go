package sql

import (
	"context"
	"database/sql/driver"
)

// ConnStmt implements driver.Stmt with a Conn.
// Construct with conn.Prepare.
type ConnStmt struct {
	conn  *Conn
	query string
}

func newConnStmt(conn *Conn, query string) *ConnStmt {
	return &ConnStmt{conn: conn, query: query}
}

// NumInput returns the number of placeholder parameters.
//
// If NumInput returns >= 0, the sql package will sanity check
// argument counts from callers and return errors to the caller
// before the statement's Exec or Query methods are called.
//
// NumInput may also return -1, if the driver doesn't know
// its number of placeholders. In that case, the sql package
// will not sanity check Exec or Query argument counts.
func (s *ConnStmt) NumInput() int {
	return -1
}

// Exec executes a query that doesn't return rows, such
// as an INSERT or UPDATE.
//
// Deprecated: Drivers should implement StmtExecContext instead (or additionally).
func (s *ConnStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.Exec(s.query, args)
}

// ExecContext executes a query that doesn't return rows, such
// as an INSERT or UPDATE.
//
// ExecContext must honor the context timeout and return when it is canceled.
func (s *ConnStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

// Query executes a query that may return rows, such as a
// SELECT.
//
// Deprecated: Drivers should implement StmtQueryContext instead (or additionally).
func (s *ConnStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.Query(s.query, args)
}

// QueryContext executes a query that may return rows, such as a
// SELECT.
//
// QueryContext must honor the context timeout and return when it is canceled.
func (s *ConnStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

// Close closes the statement.
//
// As of Go 1.1, a Stmt will not be closed if it's in use
// by any queries.
//
// Drivers must ensure all network calls made by Close
// do not block indefinitely (e.g. apply a timeout).
func (s *ConnStmt) Close() error {
	// no-op
	return nil
}

// _ is a type assertion
var (
	_ driver.Stmt             = (*ConnStmt)(nil)
	_ driver.StmtExecContext  = (*ConnStmt)(nil)
	_ driver.StmtQueryContext = (*ConnStmt)(nil)
)
