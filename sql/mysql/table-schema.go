package mysql

import (
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/pkg/errors"
)

// NewTableSchema constructs a table schema from sql schema.
func NewTableSchema(schema sql.Schema) *TableSchema {
	sch := &TableSchema{}
	cols := make([]*TableSchemaColumn, len(schema))
	for i, col := range schema {
		cols[i] = NewTableSchemaColumn(col)
	}
	sch.Columns = cols
	return sch
}

// Validate performs cursory validation of the table schema.
func (s *TableSchema) Validate() error {
	cols := s.GetColumns()
	if len(cols) == 0 {
		return ErrEmptyTable
	}
	for i, col := range s.GetColumns() {
		if err := col.Validate(); err != nil {
			return errors.Wrapf(err, "columns[%d]", i)
		}
	}
	return nil
}

// ToSqlSchema converts to a table sql schema.
//
// Ctx is optional.
func (s *TableSchema) ToSqlSchema(ctx *sql.Context) (sql.Schema, error) {
	cols := s.GetColumns()
	sch := make(sql.Schema, len(cols))
	for i, col := range cols {
		scol, err := col.ToSqlColumn(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "columns[%d]", i)
		}
		sch[i] = scol
	}
	return sch, nil
}
