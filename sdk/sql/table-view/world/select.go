//go:build !tinygo && !sql_lite

package s4wave_sql_table_view_world

import (
	"database/sql/driver"
	"strconv"
	"strings"

	"github.com/pkg/errors"
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
	args := sql_rpc.SqlValuesToNamedValues(tableView.GetWhereParameters())
	where := strings.TrimSpace(tableView.GetWhereExpression())
	if where == "" && len(args) != 0 {
		return "", nil, 0, errors.New("sql/table-view: where parameters require a where expression")
	}
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

func tableViewFetchLimit(tableView *s4wave_sql_table_view.TableView) uint32 {
	if tableView.GetRowLimit() == 0 {
		return defaultFetchRowsLimit
	}
	return tableView.GetRowLimit()
}
