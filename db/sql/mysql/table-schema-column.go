package mysql

import (
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/planbuilder"
	"github.com/dolthub/go-mysql-server/sql/types"
	"github.com/pkg/errors"
)

// NewTableSchemaColumn constructs a table column from sql schema.
func NewTableSchemaColumn(col *sql.Column) *TableSchemaColumn {
	// Preserve an absent SQL column as an absent stored column.
	if col == nil {
		return nil
	}

	// Copy the SQL column definition and its type and default expression.
	tc := &TableSchemaColumn{
		Name:          col.Name,
		AutoIncrement: col.AutoIncrement,
		Nullable:      col.Nullable,
		Source:        col.Source,
		PrimaryKey:    col.PrimaryKey,
		Comment:       col.Comment,
		Extra:         col.Extra,
		Virtual:       col.Virtual,
	}
	if col.Type != nil {
		tc.ColumnType = col.Type.String()
	}
	if col.Default != nil {
		tc.DefaultValueExpr = col.Default.String()
	} else if col.Generated != nil {
		tc.GeneratedExpr = col.Generated.String()
	}
	if col.OnUpdate != nil {
		tc.OnUpdateExpr = col.OnUpdate.String()
	}
	return tc
}

// ToSqlColumn converts the proto into a sql column.
//
// Stored expressions return as unresolved text, as Dolt loads its schemas, so
// each statement's planner binds them to the column type. A parsed expression
// has no output type and leaves every plan over the table unresolved.
func (t *TableSchemaColumn) ToSqlColumn() (*sql.Column, error) {
	// Require a stored column definition with a column name.
	if t == nil {
		return nil, ErrEmptyTableColumn
	}
	tname := t.GetName()
	if len(tname) == 0 {
		return nil, ErrEmptyTableColumnName
	}

	// Construct the SQL column from its stored attributes.
	col := &sql.Column{
		Name:          tname,
		AutoIncrement: t.GetAutoIncrement(),
		Nullable:      t.GetNullable(),
		Source:        t.GetSource(),
		PrimaryKey:    t.GetPrimaryKey(),
		Comment:       t.GetComment(),
		Extra:         t.GetExtra(),
		Virtual:       t.GetVirtual(),
	}

	// Parse the stored column type for the SQL schema.
	if colType := t.GetColumnType(); colType != "" {
		ttype, err := t.ParseColumnType()
		if err != nil {
			return nil, errors.Wrap(err, "column_type")
		}
		col.Type = ttype
	}

	// Keep the stored expressions for the planner to resolve.
	if defvexp := t.GetDefaultValueExpr(); defvexp != "" {
		col.Default = sql.NewUnresolvedColumnDefaultValue(defvexp)
	}
	if genexp := t.GetGeneratedExpr(); genexp != "" {
		col.Generated = sql.NewUnresolvedColumnDefaultValue(genexp)
	}
	if onupexp := t.GetOnUpdateExpr(); onupexp != "" {
		// Literal keeps the unparenthesized text as stored.
		col.OnUpdate = &sql.ColumnDefaultValue{
			Expr:    &sql.UnresolvedColumnDefault{ExprString: onupexp},
			Literal: true,
		}
	}

	// Require primary-key columns to reject null values.
	// NOTE: primary-key nullability is an upstream todo to fix.
	// See memory/table.go.
	if col.PrimaryKey {
		col.Nullable = false
	}
	return col, nil
}

// Validate cursorily validates the table column.
//
// Each stored expression must parse, though the planner binds it to the
// column type only when a statement uses the table.
func (t *TableSchemaColumn) Validate() error {
	if _, err := t.ToSqlColumn(); err != nil {
		return err
	}
	ctx := sql.NewEmptyContext()
	for _, f := range []struct{ name, expr string }{
		{"default_value_expr", t.GetDefaultValueExpr()},
		{"generated_expr", t.GetGeneratedExpr()},
		{"on_update_expr", t.GetOnUpdateExpr()},
	} {
		if f.expr == "" {
			continue
		}
		if _, err := planbuilder.StringToColumnDefaultValue(ctx, f.expr); err != nil {
			return errors.Wrap(err, f.name)
		}
	}
	return nil
}

// ParseColumnType parses the column type to sql type.
func (t *TableSchemaColumn) ParseColumnType() (sql.Type, error) {
	// return parse.ParseColumnTypeString(sqlContext, t.GetColumnType())
	tt, err := ParseColumnType(t.GetColumnType())
	if err != nil {
		return nil, err
	}
	return types.ColumnTypeToType(tt)
}
