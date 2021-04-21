package mysql

import (
	"context"
	"database/sql"

	sql_gorm "github.com/aperturerobotics/hydra/sql/gorm"
	gdriver "github.com/dolthub/go-mysql-server/driver"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewSqlDriver constructs a sql driver from a transaction.
func NewSqlDriver(tx *Tx, driverOpts gdriver.Options) *gdriver.Driver {
	provider := NewDriverProvider(tx)
	return gdriver.New(provider, driverOpts)
}

// NewSqlDb opens the sql database driver.
func NewSqlDb(tx *Tx) (*sql.DB, error) {
	driver := NewSqlDriver(tx, gdriver.Options{})
	// as of writing this: the dsn parsing only allows for overriding jsonAs
	var dsn string
	conn, err := driver.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(conn), nil
}

// NewMysqlGorm constructs a go-orm instance from a Mysql cursor.
// dsn allows specifying the database name and/or other parameters in the "url"
func NewMysqlGorm(ctx context.Context, le *logrus.Entry, tx *Tx, conf *gorm.Config) (*gorm.DB, *sql.DB, error) {
	sqlDb, err := NewSqlDb(tx)
	if err != nil {
		return nil, nil, err
	}
	gr, err := sql_gorm.NewGorm(le, sqlDb, conf)
	if err != nil {
		_ = sqlDb.Close()
		return nil, nil, err
	}
	return gr, sqlDb, nil
}
