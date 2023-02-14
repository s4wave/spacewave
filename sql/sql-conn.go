package sql

import (
	"context"
	"database/sql/driver"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dolthub/vitess/go/vt/sqlparser"
)

// SqlConn is the set of interfaces that Conn implements.
type SqlConn interface {
	driver.Conn
	driver.ConnBeginTx

	driver.SessionResetter
	driver.Validator
	driver.Execer
	driver.ExecerContext
	driver.Queryer
	driver.QueryerContext
}

// Conn implements sql/driver.Conn with a SqlStore.
//
// The Conn can service a single query at a time. When starting a new SQL
// transaction and/or when executing the first statement, the Conn will call
// SqlStore.NewSqlTransaction to build a SqlTransaction handle.
//
// When rolling back a sql transaction and/or resetting the conn, if non-nil,
// SqlTransaction.Discard is called to discard the transaction.
//
// Because there can be multiple underlying sql Conn instances as transactions
// are built and discarded, the database name (USE) must be remembered by Conn.
type Conn struct {
	store SqlStore
	dsn   string

	mtx      sync.Mutex
	released atomic.Bool

	storeTxCtx context.Context
	storeTx    SqlTransaction

	useStmt string
}

// NewConn constructs a new Conn.
func NewConn(store SqlStore, dsn string) *Conn {
	return &Conn{store: store, dsn: dsn}
}

// Prepare returns a prepared statement, bound to this connection.
func (c *Conn) Prepare(query string) (driver.Stmt, error) {
	return newConnStmt(c, query), nil
}

// Close invalidates and potentially stops any current
// prepared statements and transactions, marking this
// connection as no longer in use.
//
// Because the sql package maintains a free pool of
// connections and only calls Close when there's a surplus of
// idle connections, it shouldn't be necessary for drivers to
// do their own connection caching.
//
// Drivers must ensure all network calls made by Close
// do not block indefinitely (e.g. apply a timeout).
func (c *Conn) Close() error {
	c.Release()
	return nil
}

// Begin starts and returns a new transaction.
//
// Deprecated: Drivers should implement ConnBeginTx instead (or additionally).
func (c *Conn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// IsValid is called prior to placing the connection into the
// connection pool. The connection will be discarded if false is returned.
func (c *Conn) IsValid() bool {
	return !c.released.Load()
}

// ResetSession is called prior to executing a query on the connection
// if the connection has been used before. If the driver returns ErrBadConn
// the connection is discarded.
func (c *Conn) ResetSession(ctx context.Context) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.released.Load() {
		return driver.ErrBadConn
	}

	// discard ongoing tx
	if c.storeTx != nil {
		c.storeTx.Discard()
		c.storeTx, c.storeTxCtx = nil, nil
	}

	return nil
}

// BeginTx starts and returns a new transaction.
// If the context is canceled by the user the sql package will
// call Tx.Rollback before discarding and closing the connection.
//
// This must check opts.Isolation to determine if there is a set
// isolation level. If the driver does not support a non-default
// level and one is set or if there is a non-default isolation level
// that is not supported, an error must be returned.
//
// This must also check opts.ReadOnly to determine if the read-only
// value is true to either set the read-only transaction property if supported
// or return an error if it is not supported.
func (c *Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.beginTxLocked(ctx, opts)
}

// Exec executes a query that doesn't return rows, such
// as an INSERT or UPDATE.
//
// Deprecated: Drivers should implement StmtExecContext instead (or additionally).
func (c *Conn) Exec(query string, args []driver.Value) (driver.Result, error) {
	return c.ExecContext(context.Background(), query, ConvertToNamedValues(args))
}

// ExecContext executes a query in the Exec mode.
func (c *Conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	var res driver.Result
	rerr := c.performOpLocked(ctx, func(tx SqlTransaction, ops SqlOps) error {
		var err error
		res, err = ops.ExecContext(ctx, query, args)
		if err == nil {
			c.checkSwitchDatabaseLocked(query)
		}
		return err
	})
	return res, rerr
}

// Query executes a query that may return rows, such as a
// SELECT.
//
// Deprecated: Drivers should implement StmtQueryContext instead (or additionally).
func (c *Conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return c.QueryContext(context.Background(), query, ConvertToNamedValues(args))
}

// QueryContext executes a query in the Query mode.
func (c *Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	var res driver.Rows
	rerr := c.performOpLocked(ctx, func(tx SqlTransaction, ops SqlOps) error {
		var err error
		res, err = ops.QueryContext(ctx, query, args)
		if err == nil {
			c.checkSwitchDatabaseLocked(query)
		}
		return err
	})
	return res, rerr
}

// checkSwitchDatabaseLocked checks if we ran a USE statement and if so,
// remembers the database that we switched to most recently. We will need to
// re-run the USE statement when building new transactions.
//
// TODO: support SELECT DATABASE(); ?
func (c *Conn) checkSwitchDatabaseLocked(query string) {
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return
	}
	switch st := stmt.(type) {
	case *sqlparser.Use:
		dbName := strings.TrimSpace(st.DBName.String())
		if dbName != "" {
			c.useStmt = sqlparser.String(st)
		}
	}
}

// performOpLocked performs an operation with a transaction.
// caller must lock mutex
func (c *Conn) performOpLocked(ctx context.Context, op func(tx SqlTransaction, ops SqlOps) error) (rerr error) {
	// check if released
	if c.released.Load() {
		return driver.ErrBadConn
	}

	// if we are initializing a transaction, commit or discard at the end of the operation.
	storeTx := c.storeTx
	if storeTx == nil {
		_, err := c.beginTxLocked(ctx, driver.TxOptions{})
		if err != nil {
			return err
		}
		storeTx = c.storeTx
		defer func() {
			if rerr == nil {
				rerr = storeTx.Commit(ctx)
			} else {
				storeTx.Discard()
			}
			c.storeTx, c.storeTxCtx = nil, nil
		}()
	}

	// get the ops and exec the query
	ops, err := storeTx.GetSqlOps(ctx)
	if err != nil {
		return err
	}

	// apply the database switch if necessary
	if c.useStmt != "" {
		_, err = ops.ExecContext(ctx, c.useStmt, nil)
		if err != nil {
			return err
		}
	}

	return op(storeTx, ops)
}

// Release releases the conn fully.
func (c *Conn) Release() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if c.released.Swap(true) {
		return
	}

	if c.storeTx != nil {
		c.storeTx.Discard()
		c.storeTx, c.storeTxCtx = nil, nil
	}
}

// beginTxLocked begins the transaction while mtx is locked, discarding any existing tx.
func (c *Conn) beginTxLocked(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if storeTx := c.storeTx; storeTx != nil {
		storeTx.Discard()
		c.storeTx, c.storeTxCtx = nil, nil
	}
	write := !opts.ReadOnly
	stx, err := c.store.NewSqlTransaction(ctx, write, c.dsn)
	if err != nil {
		return nil, err
	}
	c.storeTx, c.storeTxCtx = stx, ctx
	return newConnTx(c, stx), nil
}

// _ is a type assertion
var _ SqlConn = ((*Conn)(nil))
