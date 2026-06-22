//go:build !js && !tinygo && !sql_lite

package s4wave_sql_world

import (
	"context"
	"database/sql/driver"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	sql_mysql "github.com/s4wave/spacewave/db/sql/mysql"
	"github.com/s4wave/spacewave/db/world"
)

func (o *SqlSetRootOp) rebaseRoot(
	ctx context.Context,
	os world.ObjectState,
	currentRoot *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	var nextRoot *bucket.ObjectRef
	err := os.AccessWorldState(ctx, currentRoot, func(root *bucket_lookup.Cursor) error {
		rootCursor := root.Clone()
		defer rootCursor.Release()
		db := sql_mysql.NewMysql(rootCursor, func(root *bucket.ObjectRef) error {
			nextRoot = root.Clone()
			return nil
		})
		dsn := ""
		if statements := o.GetStatements(); len(statements) != 0 {
			dsn = statements[0].GetDsn()
		}
		tx, err := db.NewSqlTransaction(ctx, true, dsn)
		if err != nil {
			return err
		}
		defer tx.Discard()
		ops, err := tx.GetSqlOps(ctx)
		if err != nil {
			return err
		}
		for _, statement := range o.GetStatements() {
			if statement.GetDsn() != dsn {
				return errors.New("sql/db: mixed transaction DSNs cannot be rebased")
			}
			args := sqlStatementNamedValues(statement)
			switch statement.GetKind() {
			case SqlStatementKind_SQL_STATEMENT_KIND_EXEC:
				if _, err := ops.ExecContext(ctx, statement.GetQuery(), args); err != nil {
					return err
				}
			case SqlStatementKind_SQL_STATEMENT_KIND_QUERY:
				rows, err := ops.QueryContext(ctx, statement.GetQuery(), args)
				if err != nil {
					return err
				}
				if err := drainSqlRows(rows); err != nil {
					return err
				}
			default:
				return errors.New("sql/db: invalid statement kind")
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if nextRoot == nil {
			nextRoot = db.GetRootNodeRef()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if nextRoot == nil || nextRoot.GetEmpty() {
		return nil, errors.New("sql/db: rebased root was not captured")
	}
	return nextRoot, nil
}

func drainSqlRows(rows driver.Rows) error {
	defer rows.Close()
	values := make([]driver.Value, len(rows.Columns()))
	for {
		err := rows.Next(values)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
