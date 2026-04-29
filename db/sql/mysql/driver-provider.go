package mysql

import (
	"context"
	"strconv"

	gdriver "github.com/dolthub/go-mysql-server/driver"
	"github.com/dolthub/go-mysql-server/sql"
)

// DriverProvider implements the Provider interface for the sql driver.
type DriverProvider struct {
	ctx context.Context
	sql *Tx
}

// NewDriverProvider constructs a driver provider.
//
// ctx is used for the Resolve() function
func NewDriverProvider(ctx context.Context, sqlTx *Tx) *DriverProvider {
	return &DriverProvider{ctx: ctx, sql: sqlTx}
}

// Resolve is called in OpenConnector to lookup the database with the given name.
func (p *DriverProvider) Resolve(name string, options *gdriver.Options) (string, sql.DatabaseProvider, error) {
	catalog, err := p.sql.BuildDatabaseProvider(p.ctx)
	if err != nil {
		return "", nil, err
	}
	return name, catalog, nil
}

// NewSession builds a session for the connection.
func (p *DriverProvider) NewSession(
	ctx context.Context,
	id uint32,
	conn *gdriver.Connector,
) (sql.Session, error) {
	return sql.NewBaseSessionWithClientServer(
		conn.Server(),
		sql.Client{
			// User: string,
			// Capabilities uint32,
			Address: "#" + strconv.Itoa(int(id)),
		},
		id,
	), nil
}

// NewContext constructs the SQL context for the conn.
func (p *DriverProvider) NewContext(
	ctx context.Context,
	conn *gdriver.Conn,
	opts ...sql.ContextOption,
) (*sql.Context, error) {
	dsn := conn.DSN()
	var dbName string
	if dsn != "" {
		parsed, err := parseDSN(dsn)
		if err != nil {
			return nil, err
		}
		dbName = parsed
	}

	sctx := sql.NewContext(ctx, opts...)
	if dbName != "" {
		sctx.SetCurrentDatabase(dbName)
	}
	return sctx, nil
}

// _ is a type assertion
var (
	_ gdriver.ProviderWithContextBuilder = ((*DriverProvider)(nil))
	_ gdriver.ProviderWithSessionBuilder = ((*DriverProvider)(nil))
)
