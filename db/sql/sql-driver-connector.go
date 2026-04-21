package sql

import (
	"context"
	"database/sql/driver"
)

// DriverConnector implements sql.Connector with a Driver.
type DriverConnector struct {
	driver *Driver
	dsn    string
}

// NewDriverConnector constructs a new DriverConnector from a SqlStore.
func NewDriverConnector(driver *Driver, dsn string) *DriverConnector {
	return &DriverConnector{driver: driver, dsn: dsn}
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
func (d *DriverConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return d.driver.Open(d.dsn)
}

// Driver returns the underlying Driver of the Connector,
// mainly to maintain compatibility with the Driver method
// on sql.DB.
func (d *DriverConnector) Driver() driver.Driver {
	return d.driver
}

// _ is a type assertion
// these are the sql/driver interfaces DriverConnector implements
var _ driver.Connector = ((*DriverConnector)(nil))
