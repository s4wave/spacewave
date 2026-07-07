//go:build !tinygo && !sql_lite

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
	if schema == nil {
		return "", nil, 0, errors.New("sql/table-view: target schema is required")
	}
	if tableView == nil {
		return "", nil, 0, errors.New("sql/table-view: table view is required")
	}
	schemaIdent, err := s4wave_sql.QuoteIdentifier(schema.GetSchemaName())
	if err != nil {
		return "", nil, 0, errors.Wrap(err, "sql/table-view: target schema name")
	}
	tableIdent, err := s4wave_sql.QuoteIdentifier(tableView.GetTargetTableName())
	if err != nil {
		return "", nil, 0, errors.Wrap(err, "sql/table-view: target table name")
	}
	projection, err := compileProjection(tableView.GetProjectedColumns())
	if err != nil {
		return "", nil, 0, err
	}
	orderBy, err := compileOrderBy(tableView.GetSortOrder())
	if err != nil {
		return "", nil, 0, err
	}
	where, whereParams, err := compileTableViewWhere(tableView)
	if err != nil {
		return "", nil, 0, err
	}
	args := sql_rpc.SqlValuesToNamedValues(whereParams)
	maxRows := tableViewFetchLimit(tableView)

	var query strings.Builder
	query.WriteString("SELECT ")
	query.WriteString(projection)
	query.WriteString(" FROM ")
	query.WriteString(schemaIdent)
	query.WriteByte('.')
	query.WriteString(tableIdent)
	if where != "" {
		query.WriteString(" WHERE ")
		query.WriteString(where)
	}
	if orderBy != "" {
		query.WriteString(" ORDER BY ")
		query.WriteString(orderBy)
	}
	query.WriteString(" LIMIT ")
	query.WriteString(strconv.FormatUint(uint64(maxRows)+1, 10))
	return query.String(), args, maxRows, nil
}

func compileTableViewUpdate(
	schema *s4wave_sql_schema.Schema,
	tableView *s4wave_sql_table_view.TableView,
	req *s4wave_sql_table_view.UpdateRowRequest,
) (string, []driver.NamedValue, error) {
	if schema == nil {
		return "", nil, errors.New("sql/table-view: target schema is required")
	}
	if tableView == nil {
		return "", nil, errors.New("sql/table-view: table view is required")
	}
	if len(req.GetSetColumns()) == 0 {
		return "", nil, errors.New("sql/table-view: update requires set columns")
	}
	if len(req.GetSetColumns()) != len(req.GetSetValues()) {
		return "", nil, errors.New("sql/table-view: set columns and values length mismatch")
	}
	if len(req.GetMatchColumns()) == 0 {
		return "", nil, errors.New("sql/table-view: update requires match columns")
	}
	if len(req.GetMatchColumns()) != len(req.GetMatchValues()) {
		return "", nil, errors.New("sql/table-view: match columns and values length mismatch")
	}
	schemaIdent, err := s4wave_sql.QuoteIdentifier(schema.GetSchemaName())
	if err != nil {
		return "", nil, errors.Wrap(err, "sql/table-view: target schema name")
	}
	tableIdent, err := s4wave_sql.QuoteIdentifier(tableView.GetTargetTableName())
	if err != nil {
		return "", nil, errors.Wrap(err, "sql/table-view: target table name")
	}

	where, whereParams, err := compileTableViewWhere(tableView)
	if err != nil {
		return "", nil, err
	}
	args := make([]driver.NamedValue, 0, len(req.GetSetValues())+len(whereParams)+len(req.GetMatchValues()))
	var query strings.Builder
	query.WriteString("UPDATE ")
	query.WriteString(schemaIdent)
	query.WriteByte('.')
	query.WriteString(tableIdent)
	query.WriteString(" SET ")
	for i, column := range req.GetSetColumns() {
		if i != 0 {
			query.WriteString(", ")
		}
		columnIdent, err := s4wave_sql.QuoteIdentifier(column)
		if err != nil {
			return "", nil, errors.Wrap(err, "sql/table-view: update set column")
		}
		query.WriteString(columnIdent)
		query.WriteString(" = ?")
		args = appendSqlValueArg(args, req.GetSetValues()[i])
	}
	query.WriteString(" WHERE ")
	if where != "" {
		query.WriteByte('(')
		query.WriteString(where)
		query.WriteString(") AND ")
		for _, value := range whereParams {
			args = appendSqlValueArg(args, value)
		}
	}
	for i, column := range req.GetMatchColumns() {
		if i != 0 {
			query.WriteString(" AND ")
		}
		columnIdent, err := s4wave_sql.QuoteIdentifier(column)
		if err != nil {
			return "", nil, errors.Wrap(err, "sql/table-view: update match column")
		}
		query.WriteString(columnIdent)
		if isSqlNull(req.GetMatchValues()[i]) {
			query.WriteString(" IS NULL")
			continue
		}
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
