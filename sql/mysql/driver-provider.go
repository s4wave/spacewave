package mysql

import (
	"context"
	"strconv"

	gdriver "github.com/dolthub/go-mysql-server/driver"
	"github.com/dolthub/go-mysql-server/sql"
	mysql2 "github.com/go-sql-driver/mysql"
)

// DriverProvider implements the Provider interface for the sql driver.
type DriverProvider struct {
	sql *Tx
}

// NewDriverProvider constructs a driver provider.
func NewDriverProvider(sqlTx *Tx) *DriverProvider {
	return &DriverProvider{sql: sqlTx}
}

// Resolve is called in OpenConnector to lookup the database with the given name.
func (p *DriverProvider) Resolve(name string, options *gdriver.Options) (string, sql.DatabaseProvider, error) {
	catalog, err := p.sql.BuildDatabaseProvider()
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
	if dsn != "" {
		cfg, err := mysql2.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}
		if cfg.DBName != "" {
			opts = append(opts, sql.WithInitialDatabase(cfg.DBName))
		}
	}

	return sql.NewContext(ctx, opts...), nil
}

// _ is a type assertion
var (
	_ gdriver.ProviderWithContextBuilder = ((*DriverProvider)(nil))
	_ gdriver.ProviderWithSessionBuilder = ((*DriverProvider)(nil))
)
