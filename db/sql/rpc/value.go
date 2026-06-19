package sql_rpc

import (
	"bytes"
	"database/sql/driver"
	"fmt"
	"time"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
)

// DriverValueToSqlValue converts a database/sql driver value to the canonical wire value.
func DriverValueToSqlValue(value driver.Value) (*hydra_sql.SqlValue, error) {
	switch val := value.(type) {
	case nil:
		return &hydra_sql.SqlValue{}, nil
	case int64:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_IntValue{IntValue: val},
		}, nil
	case float64:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_FloatValue{FloatValue: val},
		}, nil
	case string:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_StrValue{StrValue: val},
		}, nil
	case []byte:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_BlobValue{BlobValue: bytes.Clone(val)},
		}, nil
	case bool:
		if val {
			return &hydra_sql.SqlValue{
				Value: &hydra_sql.SqlValue_IntValue{IntValue: 1},
			}, nil
		}
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_IntValue{IntValue: 0},
		}, nil
	case time.Time:
		return &hydra_sql.SqlValue{
			Value: &hydra_sql.SqlValue_StrValue{StrValue: val.Format(time.RFC3339Nano)},
		}, nil
	}

	converted, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		return nil, err
	}
	if !driver.IsValue(converted) {
		return nil, fmt.Errorf("sql rpc: unsupported driver value type %T", value)
	}
	return DriverValueToSqlValue(converted)
}

// SqlValueToDriverValue converts the canonical wire value to a database/sql driver value.
func SqlValueToDriverValue(value *hydra_sql.SqlValue) driver.Value {
	if value == nil {
		return nil
	}
	switch val := value.GetValue().(type) {
	case *hydra_sql.SqlValue_IntValue:
		return val.IntValue
	case *hydra_sql.SqlValue_FloatValue:
		return val.FloatValue
	case *hydra_sql.SqlValue_StrValue:
		return val.StrValue
	case *hydra_sql.SqlValue_BlobValue:
		return bytes.Clone(val.BlobValue)
	default:
		return nil
	}
}

// NamedValuesToSqlValues converts positional SQL bind arguments to wire values.
func NamedValuesToSqlValues(args []driver.NamedValue) ([]*hydra_sql.SqlValue, error) {
	if len(args) == 0 {
		return nil, nil
	}
	values := make([]*hydra_sql.SqlValue, len(args))
	for i, arg := range args {
		value, err := DriverValueToSqlValue(arg.Value)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

// SqlValuesToNamedValues converts wire values to positional SQL bind arguments.
func SqlValuesToNamedValues(values []*hydra_sql.SqlValue) []driver.NamedValue {
	if len(values) == 0 {
		return nil
	}
	args := make([]driver.NamedValue, len(values))
	for i, value := range values {
		args[i] = driver.NamedValue{
			Ordinal: i + 1,
			Value:   SqlValueToDriverValue(value),
		}
	}
	return args
}

// ValuesToNamedValues converts deprecated driver.Value slices to positional named values.
func ValuesToNamedValues(values []driver.Value) []driver.NamedValue {
	if len(values) == 0 {
		return nil
	}
	args := make([]driver.NamedValue, len(values))
	for i, value := range values {
		args[i] = driver.NamedValue{
			Ordinal: i + 1,
			Value:   value,
		}
	}
	return args
}

// NamedValuesToValues converts positional named values to deprecated driver.Value slices.
func NamedValuesToValues(args []driver.NamedValue) []driver.Value {
	if len(args) == 0 {
		return nil
	}
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return values
}
