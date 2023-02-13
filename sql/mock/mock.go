package sql_mock

import (
	"context"
	"database/sql"
	"fmt"

	hydra_sql "github.com/aperturerobotics/hydra/sql"
	"github.com/sirupsen/logrus"
)

// TestSqlDB_Basic performs basic tests on a SqlDB.
func TestSqlDB_Basic(ctx context.Context, le *logrus.Entry, db hydra_sql.SqlDB) error {
	buildEngine := func() (hydra_sql.Transaction, *sql.DB, error) {
		tx, err := db.NewTransaction(true)
		if err != nil {
			return nil, nil, err
		}
		sdb, err := tx.GetDb(ctx)
		if err != nil {
			tx.Discard()
			return nil, nil, err
		}
		return tx, sdb, nil
	}

	printQuery := func(db *sql.DB, query string) (int, error) {
		le.Infof("QUERY: %s", query)
		r, err := db.Query(query)
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

	tx, e, err := buildEngine()
	if err != nil {
		return err
	}
	tableName := "test-table"
	printQuery(e, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	for i := 0; i < 3; i++ {
		printQuery(e,
			fmt.Sprintf(
				"INSERT INTO `%s` (name, email, created_at, phone_numbers) VALUES ('entry-%d', 'account-%d@email.com', NOW(), '[\"555-555-555%d\"]')",
				tableName,
				i, i, i,
			),
		)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	tx, e, err = buildEngine()
	if err != nil {
		return err
	}
	printQuery(e, fmt.Sprintf("SELECT * FROM `%s`", tableName))
	tx.Discard()
	return nil
}
