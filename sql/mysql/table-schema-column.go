package mysql

import (
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/parse"
	"github.com/dolthub/vitess/go/vt/sqlparser"
	"github.com/pkg/errors"
)

// NewTableSchemaColumn constructs a table column from sql schema.
func NewTableSchemaColumn(col *sql.Column) *TableSchemaColumn {
	if col == nil {
		return nil
	}

	tc := &TableSchemaColumn{
		Name:          col.Name,
		AutoIncrement: col.AutoIncrement,
		Nullable:      col.Nullable,
		Source:        col.Source,
		PrimaryKey:    col.PrimaryKey,
		Comment:       col.Comment,
		Extra:         col.Extra,
	}
	if col.Type != nil {
		// NOTE: this might not work properly in all cases
		tc.ColumnType = col.Type.String()
	}
	if col.Default != nil {
		if defs := col.Default.String(); defs != "" {
			tc.DefaultValueExpr = defs
		}
	}
	return tc
}

// ToSqlColumn converts the proto into a sql column.
//
// Ctx is optional
func (t *TableSchemaColumn) ToSqlColumn(ctx *sql.Context) (*sql.Column, error) {
	if t == nil {
		return nil, ErrEmptyTableColumn
	}
	tname := t.GetName()
	if len(tname) == 0 {
		return nil, ErrEmptyTableColumnName
	}
	col := &sql.Column{
		Name:          tname,
		AutoIncrement: t.GetAutoIncrement(),
		Nullable:      t.GetNullable(),
		Source:        t.GetSource(),
		PrimaryKey:    t.GetPrimaryKey(),
		Comment:       t.GetComment(),
		Extra:         t.GetExtra(),
	}
	if t.GetColumnType() != "" {
		ttype, err := t.ParseColumnType()
		if err != nil {
			return nil, errors.Wrap(err, "column_type")
		}
		col.Type = ttype
	}
	if t.GetDefaultValueExpr() != "" {
		defv, err := t.ParseDefaultValueExpr(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "default_value_expr")
		}
		col.Default = defv
	}
	// NOTE: this is an upstream todo to fix
	// see memory/table.go
	if col.PrimaryKey {
		col.Nullable = false
	}
	return col, nil
}

// Validate performs cursory validation of the table column.
func (t *TableSchemaColumn) Validate() error {
	_, err := t.ToSqlColumn(nil)
	return err
}

// ParseColumnType parses the column type to sql type.
func (t *TableSchemaColumn) ParseColumnType() (sql.Type, error) {
	ct := t.GetColumnType()
	if ct == "" {
		return sql.Null, nil
	}
	// NOTE: this might not work properly in all cases
	return sql.ColumnTypeToType(&sqlparser.ColumnType{Type: t.GetColumnType()})
}

// ParseDefaultValueExpr parses the default value expression.
//
// Returns nil if the field is empty.
// If ctx is nil, uses default context.
func (t *TableSchemaColumn) ParseDefaultValueExpr(ctx *sql.Context) (*sql.ColumnDefaultValue, error) {
	defvexp := t.GetDefaultValueExpr()
	if len(defvexp) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = sql.NewEmptyContext()
	}
	return parse.StringToColumnDefaultValue(ctx, defvexp)
}
