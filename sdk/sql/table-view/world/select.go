//go:build !tinygo

package s4wave_sql_table_view_world

import (
	"database/sql/driver"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
	s4wave_sql "github.com/s4wave/spacewave/sdk/sql"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
)

const defaultFetchRowsLimit uint32 = 1_000

func compileTableViewSelect(
	schema *s4wave_sql_schema.Schema,
	tableView *s4wave_sql_table_view.TableView,
) (string, []driver.NamedValue, uint32, error) {
	// Require the schema and table view before compiling the SELECT.
	if schema == nil {
		return "", nil, 0, errors.New("sql/table-view: target schema is required")
	}
	if tableView == nil {
		return "", nil, 0, errors.New("sql/table-view: table view is required")
	}

	// Quote the schema and table identifiers for the SELECT target.
	schemaIdent, err := s4wave_sql.QuoteIdentifier(schema.GetSchemaName())
	if err != nil {
		return "", nil, 0, errors.Wrap(err, "sql/table-view: target schema name")
	}
	tableIdent, err := s4wave_sql.QuoteIdentifier(tableView.GetTargetTableName())
	if err != nil {
		return "", nil, 0, errors.Wrap(err, "sql/table-view: target table name")
	}

	// Compile the projected columns and sort order.
	projection, err := compileProjection(tableView.GetProjectedColumns())
	if err != nil {
		return "", nil, 0, err
	}
	orderBy, err := compileOrderBy(tableView.GetSortOrder())
	if err != nil {
		return "", nil, 0, err
	}

	// Compile the table view filter and resolve its arguments and row limit.
	where, whereParams, err := compileTableViewWhere(tableView)
	if err != nil {
		return "", nil, 0, err
	}
	args := sql_rpc.SqlValuesToNamedValues(whereParams)
	maxRows := tableViewFetchLimit(tableView)

	// Build the SELECT projection and qualified table target.
	var query strings.Builder
	query.WriteString("SELECT ")
	query.WriteString(projection)
	query.WriteString(" FROM ")
	query.WriteString(schemaIdent)
	query.WriteByte('.')
	query.WriteString(tableIdent)

	// Apply the table view filter and sort order to the SELECT.
	if where != "" {
		query.WriteString(" WHERE ")
		query.WriteString(where)
	}
	if orderBy != "" {
		query.WriteString(" ORDER BY ")
		query.WriteString(orderBy)
	}

	// Limit the SELECT to one extra row so truncation can be detected.
	query.WriteString(" LIMIT ")
	query.WriteString(strconv.FormatUint(uint64(maxRows)+1, 10))

	return query.String(), args, maxRows, nil
}

func compileTableViewUpdate(
	schema *s4wave_sql_schema.Schema,
	tableView *s4wave_sql_table_view.TableView,
	req *s4wave_sql_table_view.UpdateRowRequest,
) (string, []driver.NamedValue, error) {
	// Require the schema and table view before compiling the UPDATE.
	if schema == nil {
		return "", nil, errors.New("sql/table-view: target schema is required")
	}
	if tableView == nil {
		return "", nil, errors.New("sql/table-view: table view is required")
	}

	// Require matching column and value counts for the update assignments.
	if len(req.GetSetColumns()) == 0 {
		return "", nil, errors.New("sql/table-view: update requires set columns")
	}
	if len(req.GetSetColumns()) != len(req.GetSetValues()) {
		return "", nil, errors.New("sql/table-view: set columns and values length mismatch")
	}

	// Require matching column and value counts for the row predicates.
	if len(req.GetMatchColumns()) == 0 {
		return "", nil, errors.New("sql/table-view: update requires match columns")
	}
	if len(req.GetMatchColumns()) != len(req.GetMatchValues()) {
		return "", nil, errors.New("sql/table-view: match columns and values length mismatch")
	}

	// Quote the schema and table identifiers for the UPDATE target.
	schemaIdent, err := s4wave_sql.QuoteIdentifier(schema.GetSchemaName())
	if err != nil {
		return "", nil, errors.Wrap(err, "sql/table-view: target schema name")
	}
	tableIdent, err := s4wave_sql.QuoteIdentifier(tableView.GetTargetTableName())
	if err != nil {
		return "", nil, errors.Wrap(err, "sql/table-view: target table name")
	}

	// Compile the table view filter for the UPDATE.
	where, whereParams, err := compileTableViewWhere(tableView)
	if err != nil {
		return "", nil, err
	}

	// Build the qualified UPDATE target and allocate its arguments.
	args := make([]driver.NamedValue, 0, len(req.GetSetValues())+len(whereParams)+len(req.GetMatchValues()))
	var query strings.Builder
	query.WriteString("UPDATE ")
	query.WriteString(schemaIdent)
	query.WriteByte('.')
	query.WriteString(tableIdent)
	query.WriteString(" SET ")

	// Bind each assigned column to its typed update value.
	for i, column := range req.GetSetColumns() {
		// Separate and quote the next assignment column.
		if i != 0 {
			query.WriteString(", ")
		}
		columnIdent, err := s4wave_sql.QuoteIdentifier(column)
		if err != nil {
			return "", nil, errors.Wrap(err, "sql/table-view: update set column")
		}

		// Append the assignment expression and its bound value.
		query.WriteString(columnIdent)
		query.WriteString(" = ?")
		args = appendSqlValueArg(args, req.GetSetValues()[i])
	}

	// Constrain the UPDATE to the table view's filter.
	query.WriteString(" WHERE ")
	if where != "" {
		query.WriteByte('(')
		query.WriteString(where)
		query.WriteString(") AND ")
		for _, value := range whereParams {
			args = appendSqlValueArg(args, value)
		}
	}

	// Add predicates matching the row's original column values.
	for i, column := range req.GetMatchColumns() {
		// Separate and quote the next matching column.
		if i != 0 {
			query.WriteString(" AND ")
		}
		columnIdent, err := s4wave_sql.QuoteIdentifier(column)
		if err != nil {
			return "", nil, errors.Wrap(err, "sql/table-view: update match column")
		}

		// Match null values without binding a placeholder.
		query.WriteString(columnIdent)
		if isSqlNull(req.GetMatchValues()[i]) {
			query.WriteString(" IS NULL")
			continue
		}

		// Bind the non-null matching value to its predicate.
		query.WriteString(" = ?")
		args = appendSqlValueArg(args, req.GetMatchValues()[i])
	}

	return query.String(), args, nil
}

func compileProjection(columns []string) (string, error) {
	if len(columns) == 0 {
		return "*", nil
	}
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		columnIdent, err := s4wave_sql.QuoteIdentifier(column)
		if err != nil {
			return "", errors.Wrap(err, "sql/table-view: projected column")
		}
		quoted = append(quoted, columnIdent)
	}
	return strings.Join(quoted, ", "), nil
}

func compileOrderBy(sortOrder []*s4wave_sql_table_view.SortOrder) (string, error) {
	if len(sortOrder) == 0 {
		return "", nil
	}
	terms := make([]string, 0, len(sortOrder))
	for _, sort := range sortOrder {
		if sort == nil {
			continue
		}
		columnIdent, err := s4wave_sql.QuoteIdentifier(sort.GetColumnName())
		if err != nil {
			return "", errors.Wrap(err, "sql/table-view: sort column")
		}
		direction := "ASC"
		if sort.GetDescending() {
			direction = "DESC"
		}
		terms = append(terms, columnIdent+" "+direction)
	}
	return strings.Join(terms, ", "), nil
}

func compileTableViewWhere(tableView *s4wave_sql_table_view.TableView) (string, []*hydra_sql.SqlValue, error) {
	where := strings.TrimSpace(tableView.GetWhereExpression())
	params := tableView.GetWhereParameters()
	if where == "" && len(params) != 0 {
		return "", nil, errors.New("sql/table-view: where parameters require a where expression")
	}
	return where, params, nil
}

func tableViewFetchLimit(tableView *s4wave_sql_table_view.TableView) uint32 {
	if tableView.GetRowLimit() == 0 {
		return defaultFetchRowsLimit
	}
	return tableView.GetRowLimit()
}

func appendSqlValueArg(args []driver.NamedValue, value *hydra_sql.SqlValue) []driver.NamedValue {
	return append(args, driver.NamedValue{
		Ordinal: len(args) + 1,
		Value:   sql_rpc.SqlValueToDriverValue(value),
	})
}

func isSqlNull(value *hydra_sql.SqlValue) bool {
	return value == nil || value.GetValue() == nil
}
