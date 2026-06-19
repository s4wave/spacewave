package sql_rpc

import (
	"bytes"
	"database/sql/driver"
	"testing"
	"time"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
)

func TestDriverValueToSqlValueCoversDriverNativeTypes(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 34, 56, 789, time.UTC)
	tests := []struct {
		name  string
		value driver.Value
		want  driver.Value
	}{
		{name: "null", value: nil, want: nil},
		{name: "int64", value: int64(42), want: int64(42)},
		{name: "float64", value: float64(3.25), want: float64(3.25)},
		{name: "string", value: "alice", want: "alice"},
		{name: "bytes", value: []byte{1, 2, 3}, want: []byte{1, 2, 3}},
		{name: "bool true", value: true, want: int64(1)},
		{name: "bool false", value: false, want: int64(0)},
		{name: "time", value: now, want: now.Format(time.RFC3339Nano)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wireValue, err := DriverValueToSqlValue(tt.value)
			if err != nil {
				t.Fatalf("DriverValueToSqlValue: %v", err)
			}
			got := SqlValueToDriverValue(wireValue)
			if gotBytes, ok := got.([]byte); ok {
				wantBytes := tt.want.([]byte)
				if !bytes.Equal(gotBytes, wantBytes) {
					t.Fatalf("round trip bytes = %v, want %v", gotBytes, wantBytes)
				}
				gotBytes[0] = 9
				if bytes.Equal(gotBytes, wantBytes) {
					t.Fatal("SqlValueToDriverValue returned aliased bytes")
				}
				return
			}
			if got != tt.want {
				t.Fatalf("round trip = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSqlValuesToNamedValuesAssignsOrdinals(t *testing.T) {
	args := SqlValuesToNamedValues([]*hydra_sql.SqlValue{
		{Value: &hydra_sql.SqlValue_IntValue{IntValue: 1}},
		{Value: &hydra_sql.SqlValue_StrValue{StrValue: "two"}},
	})
	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if args[0].Ordinal != 1 || args[1].Ordinal != 2 {
		t.Fatalf("ordinals = %d, %d; want 1, 2", args[0].Ordinal, args[1].Ordinal)
	}
}
