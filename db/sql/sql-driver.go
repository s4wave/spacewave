package sql

import (
	"context"
	"database/sql/driver"
)

// Driver implements sql.Driver with a common SqlStore.
type Driver struct {
	store SqlStore
	dsn   string
}

// NewDriver constructs a new Driver from a SqlStore.
//
// dsn is the default database name string.
func NewDriver(store SqlStore, dsn string) *Driver {
	return &Driver{store: store, dsn: dsn}
}

// Open returns a new connection to the database.
// The name is a string in a driver-specific format.
//
// Open may return a cached connection (one previously
// closed), but doing so is unnecessary; the sql package
// maintains a pool of idle connections for efficient re-use.
//
// The returned connection is only used by one goroutine at a
// time.
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	return NewConn(d.store, dsn), nil
}

// OpenConnector must parse the name in the same format that Driver.Open
// parses the name parameter.
func (d *Driver) OpenConnector(dsn string) (driver.Connector, error) {
	return NewDriverConnector(d, dsn), nil
}

// Connect returns a connection to the database.
// Connect may return a cached connection (one previously
// closed), but doing so is unnecessary; the sql package
// maintains a pool of idle connections for efficient re-use.
//
// The provided context.Context is for dialing purposes only
// (see net.DialContext) and should not be stored or used for
// other purposes. A default timeout should still be used
// when dialing as a connection pool may call Connect
// asynchronously to any query.
//
// The returned connection is only used by one goroutine at a
// time.
func (d *Driver) Connect(ctx context.Context) (driver.Conn, error) {
	return d.Open(d.dsn)
}

// Driver returns the underlying Driver of the Connector,
// mainly to maintain compatibility with the Driver method
// on sql.DB.
func (d *Driver) Driver() driver.Driver {
	return d
}

// _ is a type assertion
// these are the sql/driver interfaces Driver implements
var (
	_ driver.Connector     = (*Driver)(nil)
	_ driver.Driver        = (*Driver)(nil)
	_ driver.DriverContext = (*Driver)(nil)
)

// NOTE: from sql/driver package:
//
//   The Connector.Connect and Driver.Open methods should never return ErrBadConn.
//   ErrBadConn should only be returned from Validator, SessionResetter, or
//   a query method if the connection is already in an invalid (e.g. closed) state.
