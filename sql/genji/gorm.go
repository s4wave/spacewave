package kvtx_genji

import (
	"context"
	"database/sql"
	"database/sql/driver"

	"github.com/aperturerobotics/hydra/kvtx"
	sql_gorm "github.com/aperturerobotics/hydra/sql/gorm"
	gdriver "github.com/genjidb/genji/driver"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewKvtxGorm constructs a go-orm instance from a kvtx store.
func NewKvtxGorm(ctx context.Context, le *logrus.Entry, store kvtx.Store, conf *gorm.Config) (*gorm.DB, error) {
	// NOTE genji is not sql feature complete
	sdb, err := NewGenjiDB(ctx, store)
	if err != nil {
		return nil, err
	}
	driver, ok := gdriver.NewDriver(sdb).(driver.DriverContext)
	if !ok {
		return nil, gorm.ErrNotImplemented
	}
	conn, err := driver.OpenConnector("")
	if err != nil {
		return nil, err
	}
	sqlDB := sql.OpenDB(conn)
	return sql_gorm.NewGorm(le, sqlDB, &gorm.Config{})
}
