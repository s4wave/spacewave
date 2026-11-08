//go:build !js && !wasip1

package s4wave_sql_world

import (
	"database/sql/driver"
	"io"
	"time"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/vitess/go/mysql"
	"github.com/dolthub/vitess/go/sqltypes"
	querypb "github.com/dolthub/vitess/go/vt/proto/query"
)

// mysqlRowsBatch bounds live protocol rows and marks a full intermediate batch.
const mysqlRowsBatch = 128

// streamMysqlRows applies network backpressure after each bounded row batch.
func streamMysqlRows(rows driver.Rows, callback mysql.ResultSpoolFn) (rerr error) {
	// Close the iterator on read failure, disconnect and successful completion.
	defer func() {
		if err := rows.Close(); rerr == nil {
			rerr = err
		}
	}()
	columns := rows.Columns()
	fields := make([]*querypb.Field, len(columns))
	for i, name := range columns {
		fields[i] = &querypb.Field{Name: name, Type: querypb.Type_TEXT, Charset: uint32(sql.Collation_utf8mb4_general_ci)}
		if typed, ok := rows.(driver.RowsColumnTypeDatabaseTypeName); ok {
			fields[i].Type = mysqlColumnType(typed.ColumnTypeDatabaseTypeName(i))
		}
	}
	values := make([]driver.Value, len(columns))

	// Keep only the current batch and the engine iterator's current position.
	for {
		result := &sqltypes.Result{Fields: fields, Rows: make([][]sqltypes.Value, 0, mysqlRowsBatch)}
		for len(result.Rows) < mysqlRowsBatch {
			if err := rows.Next(values); err != nil {
				if err == io.EOF {
					return callback(result, false)
				}
				return err
			}
			row := make([]sqltypes.Value, len(values))
			for i, value := range values {
				v, err := mysqlValue(value, fields[i].Type)
				if err != nil {
					return err
				}
				row[i] = v
			}
			result.Rows = append(result.Rows, row)
		}
		if err := callback(result, false); err != nil {
			return err
		}
	}
}

// mysqlValue encodes one driver cell using the result column's wire type.
func mysqlValue(value driver.Value, typ querypb.Type) (sqltypes.Value, error) {
	// SQL NULL carries its own type regardless of the column's declared type.
	if value == nil {
		return sqltypes.NULL, nil
	}
	if t, ok := value.(time.Time); ok {
		value = t.Format("2006-01-02 15:04:05.999999")
	}

	// BuildBindVariable handles numbers and copies only this cell's wire bytes.
	v, err := sqltypes.BuildBindVariable(value)
	if err != nil {
		return sqltypes.NULL, err
	}
	return sqltypes.NewValue(typ, v.Value)
}

// mysqlColumnTypes maps the native driver's database type names to wire types.
var mysqlColumnTypes = map[string]querypb.Type{
	"TINYINT": querypb.Type_INT8, "SMALLINT": querypb.Type_INT16,
	"MEDIUMINT": querypb.Type_INT24, "INT": querypb.Type_INT32, "BIGINT": querypb.Type_INT64,
	"UNSIGNED TINYINT": querypb.Type_UINT8, "UNSIGNED SMALLINT": querypb.Type_UINT16,
	"UNSIGNED MEDIUMINT": querypb.Type_UINT24, "UNSIGNED INT": querypb.Type_UINT32, "UNSIGNED BIGINT": querypb.Type_UINT64,
	"FLOAT": querypb.Type_FLOAT32, "DOUBLE": querypb.Type_FLOAT64, "DECIMAL": querypb.Type_DECIMAL,
	"YEAR": querypb.Type_YEAR, "DATE": querypb.Type_DATE, "TIME": querypb.Type_TIME,
	"DATETIME": querypb.Type_DATETIME, "TIMESTAMP": querypb.Type_TIMESTAMP,
	"CHAR": querypb.Type_CHAR, "VARCHAR": querypb.Type_VARCHAR, "TEXT": querypb.Type_TEXT,
	"BINARY": querypb.Type_BINARY, "VARBINARY": querypb.Type_VARBINARY, "BLOB": querypb.Type_BLOB,
	"BIT": querypb.Type_BIT, "ENUM": querypb.Type_ENUM, "SET": querypb.Type_SET, "JSON": querypb.Type_JSON,
}

// mysqlColumnType preserves the native engine's result type on the wire.
func mysqlColumnType(name string) querypb.Type {
	if typ, ok := mysqlColumnTypes[name]; ok {
		return typ
	}
	return querypb.Type_TEXT
}
