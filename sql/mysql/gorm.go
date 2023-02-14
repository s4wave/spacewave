package mysql

import (
	"context"
	"database/sql"

	sql_gorm "github.com/aperturerobotics/hydra/sql/gorm"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewMysqlGorm constructs a go-orm instance from a Mysql transaction.
// dsn allows specifying the database name and/or other parameters
func NewMysqlGorm(ctx context.Context, le *logrus.Entry, tx *Tx, conf *gorm.Config, dsn string) (*gorm.DB, *sql.DB, error) {
	sqlDb, err := NewSqlDb(tx, dsn)
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
