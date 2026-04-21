package sql_mock

import (
	"context"
	"database/sql"
	"fmt"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
	"github.com/sirupsen/logrus"
)

// TestSqlStore_Basic performs basic tests on a SqlDB.
func TestSqlStore_Basic(ctx context.Context, le *logrus.Entry, db hydra_sql.SqlStore, dsn string) error {
	sqlDB := hydra_sql.NewSqlDb(db, dsn)
	printQuery := func(tx *sql.Tx, query string) (int, error) {
		le.Infof("QUERY: %s", query)
		var r *sql.Rows
		var err error
		if tx != nil {
			r, err = tx.Query(query)
		} else {
			r, err = sqlDB.Query(query)
		}
		if err != nil {
			return 0, err
		}
		var nrows int
		for r.Next() {
			if r.Err() != nil {
				return 0, err
			}
			nrows++
			cols, err := r.Columns()
			if err != nil {
				return 0, err
			}
			for ci, col := range cols {
				le.Infof("COL %d: %v", ci, col)
			}
			/*
				for ci, col := range row {
					le.Infof("COL %d: %v", ci, col)
				} */
		}
		le.Infof("END QUERY: %d rows", nrows)
		return nrows, r.Close()
	}

	tableName := "test-table"
	_, err := printQuery(nil, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	if err != nil {
		return err
	}

	tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	for i := range 3 {
		_, err = printQuery(
			tx,
			fmt.Sprintf(
				"INSERT INTO `%s` (name, email, created_at, phone_numbers) VALUES ('entry-%d', 'account-%d@email.com', NOW(), '[\"555-555-555%d\"]')",
				tableName,
				i, i, i,
			),
		)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	_, err = printQuery(nil, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	return err
}
