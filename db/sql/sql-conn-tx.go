package sql

import (
	"database/sql/driver"

	"github.com/s4wave/spacewave/db/tx"
)

// ConnTx implements driver.Tx attached to a Conn.
// NOTE: you should use BeginTx to construct this.
type ConnTx struct {
	conn *Conn
	tx   SqlTransaction
}

// newConnTx constructs a new ConnTx.
func newConnTx(conn *Conn, tx SqlTransaction) *ConnTx {
	return &ConnTx{conn: conn, tx: tx}
}

func (c *ConnTx) Commit() error {
	c.conn.mtx.Lock()
	defer c.conn.mtx.Unlock()
	storeTx := c.conn.storeTx
	if storeTx != c.tx {
		c.tx.Discard()
		return tx.ErrDiscarded
	}
	err := storeTx.Commit(c.conn.storeTxCtx)
	c.conn.storeTx = nil
	c.conn.storeTxCtx = nil
	return err
}

func (c *ConnTx) Rollback() error {
	c.conn.mtx.Lock()
	defer c.conn.mtx.Unlock()
	storeTx := c.conn.storeTx
	if storeTx != c.tx {
		c.tx.Discard()
		return tx.ErrDiscarded
	}
	storeTx.Discard()
	c.conn.storeTx = nil
	c.conn.storeTxCtx = nil
	return nil
}

// _ is a type assertion
var _ driver.Tx = ((*ConnTx)(nil))
