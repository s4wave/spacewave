package sql

import "database/sql/driver"

// ConvertToNamedValues converts the driver.Value slice to a driver.NamedValue slice.
func ConvertToNamedValues(values []driver.Value) []driver.NamedValue {
	namedValues := make([]driver.NamedValue, len(values))
	for i, value := range values {
		namedValues[i] = driver.NamedValue{
			Ordinal: i + 1,
			Value:   value,
		}
	}
	return namedValues
}
