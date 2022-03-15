package mysql

import (
	gdriver "github.com/dolthub/go-mysql-server/driver"
	"github.com/dolthub/go-mysql-server/sql"
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

// _ is a type assertion
var _ gdriver.Provider = ((*DriverProvider)(nil))
