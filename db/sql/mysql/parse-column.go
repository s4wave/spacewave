package mysql

import (
	"context"
	"strings"

	"github.com/dolthub/vitess/go/vt/sqlparser"
	"github.com/pkg/errors"
)

// ParseColumnType parses a column type string to a sqlparser.ColumnType.
func ParseColumnType(typeStr string) (*sqlparser.ColumnType, error) {
	// Require a column type before constructing its parser input.
	if len(typeStr) == 0 {
		return nil, errors.New("column_type: empty string")
	}

	// Embed the column type in a single-column CREATE TABLE statement.
	// TODO: is there a "correct" way to do this?
	var toParse strings.Builder
	_, _ = toParse.WriteString("CREATE TABLE t (v ")
	_, _ = toParse.WriteString(typeStr)
	_, _ = toParse.WriteString(")")

	// Parse the synthetic table statement and require its single column definition.
	stmt, _, err := sqlparser.ParseOne(context.Background(), toParse.String())
	if err != nil {
		return nil, err
	}
	ddl, ok := stmt.(*sqlparser.DDL)
	if !ok {
		return nil, errors.New("unexpected non-ddl statement while parsing column type")
	}
	if ddl.TableSpec == nil || len(ddl.TableSpec.Columns) != 1 {
		return nil, errors.New("unexpected table spec while parsing column type")
	}

	// Return the parsed type from the synthetic column definition.
	colType := ddl.TableSpec.Columns[0].Type
	return &colType, nil
}
