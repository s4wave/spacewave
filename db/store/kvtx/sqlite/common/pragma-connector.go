package common

import (
	"context"
	"database/sql/driver"
	"errors"
)

// PragmaConnector opens each connection of a pool through a driver and runs
// the connection-scoped pragmas on it before the pool hands it out. SQLite
// keeps settings such as synchronous, cache_size and mmap_size per
// connection, so a pragma run once through the pool reaches only the
// connection that ran it.
type PragmaConnector struct {
	// connector opens the driver's connections.
	connector driver.Connector
	// pragmas run on every new connection, in order.
	pragmas []string
}

// NewPragmaConnector returns a connector that opens dsn through drv and runs
// pragmas on each new connection.
func NewPragmaConnector(drv driver.Driver, dsn string, pragmas []string) (*PragmaConnector, error) {
	// Use the driver's own connector when it has one, so its open options apply.
	var connector driver.Connector = dsnConnector{driver: drv, dsn: dsn}
	if dc, ok := drv.(driver.DriverContext); ok {
		c, err := dc.OpenConnector(dsn)
		if err != nil {
			return nil, err
		}
		connector = c
	}
	return &PragmaConnector{connector: connector, pragmas: pragmas}, nil
}

// Connect opens a connection and runs the pragmas on it.
func (c *PragmaConnector) Connect(ctx context.Context) (driver.Conn, error) {
	// Open the driver connection.
	conn, err := c.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}

	// Run each pragma, closing the connection if one fails.
	for _, pragma := range c.pragmas {
		if err := execConn(ctx, conn, pragma); err != nil {
			return nil, errors.Join(err, conn.Close())
		}
	}
	return conn, nil
}

// Driver returns the underlying driver.
func (c *PragmaConnector) Driver() driver.Driver {
	return c.connector.Driver()
}

// execConn runs a statement without arguments on a driver connection.
func execConn(ctx context.Context, conn driver.Conn, query string) error {
	// Prefer the driver's direct execution.
	if execer, ok := conn.(driver.ExecerContext); ok {
		_, err := execer.ExecContext(ctx, query, nil)
		return err
	}

	// Fall back to a prepared statement.
	stmt, err := conn.Prepare(query)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(nil) //nolint:staticcheck
	return errors.Join(err, stmt.Close())
}

// dsnConnector opens connections of a driver without a connector of its own.
type dsnConnector struct {
	// driver opens the connections.
	driver driver.Driver
	// dsn names the database.
	dsn string
}

// Connect opens a connection to the data source.
func (c dsnConnector) Connect(context.Context) (driver.Conn, error) {
	return c.driver.Open(c.dsn)
}

// Driver returns the driver.
func (c dsnConnector) Driver() driver.Driver {
	return c.driver
}

// _ is a type assertion
var (
	_ driver.Connector = (*PragmaConnector)(nil)
	_ driver.Connector = dsnConnector{}
)
