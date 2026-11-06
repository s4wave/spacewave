package common

import (
	"context"
	"database/sql/driver"
	"errors"
)

// stmtCacheConn keeps one prepared statement per query text on a driver
// connection, so a query the store runs on every read, such as a key lookup,
// parses its SQL once per connection instead of once per call. Statements
// that return no rows run uncached. The pool hands
// a connection to one caller at a time, so the cache needs no lock.
//
// A cached statement serves one open result at a time: a query whose
// statement still has open rows, such as a nested lookup during a scan,
// prepares a statement of its own that closes with its rows.
type stmtCacheConn struct {
	// Conn is the driver connection.
	driver.Conn
	// stmts holds the prepared statement of each query run on the connection.
	stmts map[string]*cachedStmt
}

// cachedStmt is one prepared statement and whether rows of it are open.
type cachedStmt struct {
	// stmt is the prepared statement.
	stmt driver.Stmt
	// busy is set while rows of stmt are open.
	busy bool
}

// newStmtCacheConn wraps conn with a statement cache when it supports the
// context methods the cache runs statements through; otherwise it returns
// conn unchanged.
func newStmtCacheConn(conn driver.Conn) driver.Conn {
	if _, ok := conn.(driver.ConnPrepareContext); !ok {
		return conn
	}
	return &stmtCacheConn{Conn: conn, stmts: make(map[string]*cachedStmt)}
}

// cached returns the idle cached statement of query, preparing it on first
// use, or nil when its statement has open rows or does not take contexts.
func (c *stmtCacheConn) cached(ctx context.Context, query string) (*cachedStmt, error) {
	// Reuse the idle statement.
	if st, ok := c.stmts[query]; ok {
		if st.busy {
			return nil, nil
		}
		return st, nil
	}

	// Prepare the statement once for the connection.
	stmt, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	if _, ok := stmt.(driver.StmtQueryContext); !ok {
		return nil, stmt.Close()
	}
	st := &cachedStmt{stmt: stmt}
	c.stmts[query] = st
	return st, nil
}

// QueryContext runs query through its cached statement, or through the
// connection when that statement is busy. Closing the rows resets the
// statement.
func (c *stmtCacheConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	st, err := c.cached(ctx, query)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return c.uncachedQuery(ctx, query, args)
	}
	rows, err := st.stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
	if err != nil {
		return nil, err
	}
	st.busy = true
	return &cachedRows{Rows: rows, stmt: st}, nil
}

// ExecContext runs query on the connection, outside the cache: the driver
// leaves an executed statement unreset, and a cached one would stay in
// progress and block the transaction's commit.
func (c *stmtCacheConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := c.Conn.(driver.ExecerContext); ok {
		return execer.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

// uncachedQuery runs query without the cache.
func (c *stmtCacheConn) uncachedQuery(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := c.Conn.(driver.QueryerContext); ok {
		return queryer.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

// PrepareContext prepares a statement the caller owns, outside the cache.
func (c *stmtCacheConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}

// BeginTx begins a transaction on the connection.
func (c *stmtCacheConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		return beginner.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck
}

// Ping checks the connection when the driver can.
func (c *stmtCacheConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

// ResetSession resets the connection's session when the driver can.
func (c *stmtCacheConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.Conn.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

// IsValid reports whether the pool may reuse the connection.
func (c *stmtCacheConn) IsValid() bool {
	if validator, ok := c.Conn.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

// CheckNamedValue converts an argument as the driver does, or leaves it to
// the default conversion.
func (c *stmtCacheConn) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return driver.ErrSkip
}

// Close closes the cached statements, then the connection.
func (c *stmtCacheConn) Close() error {
	errs := make([]error, 0, len(c.stmts)+1)
	for _, st := range c.stmts {
		errs = append(errs, st.stmt.Close())
	}
	c.stmts = nil
	errs = append(errs, c.Conn.Close())
	return errors.Join(errs...)
}

// cachedRows are the open rows of a cached statement; closing them frees the
// statement for the next query.
type cachedRows struct {
	// Rows are the statement's rows.
	driver.Rows
	// stmt is the statement the rows belong to.
	stmt *cachedStmt
}

// Close closes the rows and frees their statement.
func (r *cachedRows) Close() error {
	err := r.Rows.Close()
	r.stmt.busy = false
	return err
}

// _ is a type assertion
var (
	_ driver.QueryerContext     = (*stmtCacheConn)(nil)
	_ driver.ExecerContext      = (*stmtCacheConn)(nil)
	_ driver.ConnPrepareContext = (*stmtCacheConn)(nil)
	_ driver.ConnBeginTx        = (*stmtCacheConn)(nil)
	_ driver.Pinger             = (*stmtCacheConn)(nil)
	_ driver.SessionResetter    = (*stmtCacheConn)(nil)
	_ driver.Validator          = (*stmtCacheConn)(nil)
	_ driver.NamedValueChecker  = (*stmtCacheConn)(nil)
)
