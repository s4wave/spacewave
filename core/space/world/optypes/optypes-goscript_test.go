//go:build goscript

package optypes

import (
	"testing"
)

func TestLookupWorldOpExcludesSqlSetRootOpsUnderGoScript(t *testing.T) {
	for _, opTypeID := range []string{
		"sql/db/set-root",
		"sql/query/set-root",
		"sql/query-result/set-root",
		"sql/schema/set-root",
		"sql/table-view/set-root",
		"sql/workbench/set-root",
	} {
		for _, lookup := range []struct {
			name string
			fn   func(string) bool
		}{
			{
				name: "LookupWorldOp",
				fn: func(opTypeID string) bool {
					op, err := LookupWorldOp(t.Context(), opTypeID)
					if err != nil {
						t.Fatalf("LookupWorldOp(%s): %v", opTypeID, err)
					}
					return op != nil
				},
			},
			{
				name: "BuildSpaceLookupOp",
				fn: func(opTypeID string) bool {
					op, err := BuildSpaceLookupOp(nil, nil, "space/local/test")(t.Context(), opTypeID)
					if err != nil {
						t.Fatalf("BuildSpaceLookupOp(%s): %v", opTypeID, err)
					}
					return op != nil
				},
			},
		} {
			if lookup.fn(opTypeID) {
				t.Fatalf("%s(%s) resolved a SQL op that belongs to spacewave-sql", lookup.name, opTypeID)
			}
		}
	}
}
