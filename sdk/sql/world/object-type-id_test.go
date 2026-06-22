package s4wave_sql_world_test

import (
	"context"
	"testing"

	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

// TestSqlDbTypeIDResolvesWithoutFactory proves the SQL database type id and the
// SqlSetRootOp world operation resolve without the SRPC-server-backed factory.
// This file carries no build constraint, so the assertion compiles under the
// TinyGo and sql_lite browser builds that exclude objecttype.go; it guards the
// regression where SqlDbTypeID lived in that gated file and went undefined in
// the unconstrained SqlSetRootOp under -tags=tinygo.
func TestSqlDbTypeIDResolvesWithoutFactory(t *testing.T) {
	if s4wave_sql_world.SqlDbTypeID != "sql/db" {
		t.Fatalf("unexpected SqlDbTypeID %q", s4wave_sql_world.SqlDbTypeID)
	}

	op, err := s4wave_sql_world.LookupSqlSetRootOp(context.Background(), s4wave_sql_world.SqlSetRootOpId)
	if err != nil {
		t.Fatalf("lookup sql set root op: %v", err)
	}
	if op == nil {
		t.Fatal("sql set root op not found")
	}
	if op.GetOperationTypeId() != s4wave_sql_world.SqlSetRootOpId {
		t.Fatalf("unexpected op type id %q", op.GetOperationTypeId())
	}
}
