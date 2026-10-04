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
	driver.ExecerContext
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
	// Reset the session state under the connection mutex.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Reject a released connection before resetting its session.
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
	// Execute the query under a store transaction and track database switches.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Run the operation against the store transaction.
	var res driver.Result
	auto, err := c.performOpLocked(ctx, func(tx SqlTransaction, ops SqlOps) error {
		var err error
		res, err = ops.ExecContext(ctx, query, args)
		if err == nil {
			c.checkSwitchDatabaseLocked(query)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	// Commit the auto transaction now that the result is complete.
	if auto != nil {
		if err := c.endAutoTxLocked(auto, true); err != nil {
			return nil, err
		}
	}
	return res, nil
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
	// Run the query under a store transaction and track database switches.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Run the operation against the store transaction.
	var res driver.Rows
	auto, err := c.performOpLocked(ctx, func(tx SqlTransaction, ops SqlOps) error {
		var err error
		res, err = ops.QueryContext(ctx, query, args)
		if err == nil {
			c.checkSwitchDatabaseLocked(query)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	// The rows read from the auto transaction, so it ends when they close.
	if auto != nil {
		return &autoTxRows{Rows: res, conn: c, tx: auto}, nil
	}
	return res, nil
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

// performOpLocked performs an operation with the conn's transaction, opening an
// auto transaction when the conn has none. On success it returns the auto
// transaction, which the caller must end with endAutoTxLocked once the result
// is no longer read, or nil when the operation ran in an explicit transaction.
// On failure it discards the auto transaction. The caller must lock mtx.
func (c *Conn) performOpLocked(
	ctx context.Context,
	op func(tx SqlTransaction, ops SqlOps) error,
) (auto SqlTransaction, rerr error) {
	// Reject a released connection.
	if c.released.Load() {
		return nil, driver.ErrBadConn
	}

	// Open an auto transaction when there is none, discarding it on failure.
	storeTx := c.storeTx
	if storeTx == nil {
		if _, err := c.beginTxLocked(ctx, driver.TxOptions{}); err != nil {
			return nil, err
		}
		storeTx, auto = c.storeTx, c.storeTx
		defer func() {
			if rerr != nil {
				_ = c.endAutoTxLocked(storeTx, false)
			}
		}()
	}

	// Get the ops and replay the database switch on the transaction.
	ops, err := storeTx.GetSqlOps(ctx)
	if err != nil {
		return nil, err
	}
	if c.useStmt != "" {
		if _, err := ops.ExecContext(ctx, c.useStmt, nil); err != nil {
			return nil, err
		}
	}

	// Run the operation and hand back the auto transaction for the caller to end.
	if err := op(storeTx, ops); err != nil {
		return nil, err
	}
	return auto, nil
}

// endAutoTxLocked commits the auto transaction tx when ok is set and discards
// it otherwise. It does nothing when tx is no longer the conn's transaction,
// because whatever replaced it already discarded it. The caller must lock mtx.
func (c *Conn) endAutoTxLocked(tx SqlTransaction, ok bool) error {
	// Skip a transaction that was already replaced or discarded.
	if c.storeTx != tx {
		return nil
	}

	// Detach the transaction, then commit or discard it.
	ctx := c.storeTxCtx
	c.storeTx, c.storeTxCtx = nil, nil
	if !ok {
		tx.Discard()
		return nil
	}
	return tx.Commit(ctx)
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
	// Discard any existing transaction and open a new store transaction.
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

// autoTxRows are the rows of a query run in an auto transaction. The
// transaction stays open while the rows are read and ends when they close.
// database/sql holds the conn for the rows' lifetime, so no other operation
// can use the transaction meanwhile.
type autoTxRows struct {
	driver.Rows
	conn *Conn
	tx   SqlTransaction
}

// ColumnTypeDatabaseTypeName returns the database type name of a column, or
// an empty string when the inner rows do not report one.
func (r *autoTxRows) ColumnTypeDatabaseTypeName(index int) string {
	if rows, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return rows.ColumnTypeDatabaseTypeName(index)
	}
	return ""
}

// Close closes the rows and then ends the auto transaction, committing it
// when the rows closed cleanly.
func (r *autoTxRows) Close() error {
	// Close the inner rows before the transaction they read from.
	err := r.Rows.Close()

	// End the transaction, reporting the first error.
	r.conn.mtx.Lock()
	defer r.conn.mtx.Unlock()
	if terr := r.conn.endAutoTxLocked(r.tx, err == nil); err == nil {
		err = terr
	}
	return err
}

// _ is a type assertion
var (
	_ SqlConn                               = (*Conn)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*autoTxRows)(nil)
)
