//go:build !js

// Package gormadapter bridges a Mysql transaction to a gorm instance. It lives
// in its own package so importing db/sql/mysql does not drag gorm.io/gorm into
// the dependency closure; only callers that actually need gorm import this.
package gormadapter

import (
	"context"
	"database/sql"

	sql_gorm "github.com/s4wave/spacewave/db/sql/gorm"
	"github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewMysqlGorm constructs a gorm instance from a Mysql transaction. dsn allows
// specifying the database name and/or other parameters.
func NewMysqlGorm(ctx context.Context, le *logrus.Entry, tx *mysql.Tx, conf *gorm.Config, dsn string) (*gorm.DB, *sql.DB, error) {
	sqlDb, err := mysql.NewSqlDb(ctx, tx, dsn)
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
