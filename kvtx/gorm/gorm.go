package kvtx_gorm

import (
	"database/sql"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewGorm constructs a go-orm instance.
func NewGorm(le *logrus.Entry, sqlDB *sql.DB, conf *gorm.Config) (*gorm.DB, error) {
	dialector := NewDialector(sqlDB)
	if conf == nil {
		conf = &gorm.Config{Dialector: dialector, ConnPool: sqlDB}
	} else {
		conf.Dialector = dialector
		conf.ConnPool = sqlDB
	}
	if le != nil {
		conf.Logger = NewLogger(le)
	}
	return gorm.Open(dialector, conf)
}
