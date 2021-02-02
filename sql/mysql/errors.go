package mysql

import (
	"errors"

	"github.com/dolthub/go-mysql-server/sql"
)

var (
	// ErrEmptyDatabaseName is returned if an empty string is found where a db name is expected.
	ErrEmptyDatabaseName = errors.New("empty database name is invalid")
	// ErrDatabaseExists is returned if the database already exists.
	ErrDatabaseExists = errors.New("database already exists")
	// ErrDatabaseNotFound is returned if the database does not exist.
	ErrDatabaseNotFound = sql.ErrDatabaseNotFound
	// ErrEmptyTableName is returned if an empty string is found where a table name is expected.
	ErrEmptyTableName = errors.New("empty table name is invalid")
	// ErrEmptyTable is returned if an empty table is found where at least one column is expected.
	ErrEmptyTable = errors.New("empty table with no columns is invalid")
	// ErrEmptyTableColumn is returned if an empty table column is found where one is expected.
	ErrEmptyTableColumn = errors.New("empty table column schema is invalid")
	// ErrEmptyTableColumnName is returned if an empty table column name is found where one is expected.
	ErrEmptyTableColumnName = errors.New("empty table column name is invalid")
	// ErrUnexpectedType is returned if a type assertion failed.
	ErrUnexpectedType = errors.New("sql db unexpected object type")
)
