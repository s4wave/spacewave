//go:build goscript

package optypes

import (
	"testing"

	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

func TestBuildSpaceLookupOpAndLookupWorldOpResolveSqlSetRootOpsUnderGoScript(t *testing.T) {
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_world.SqlSetRootOp](t, s4wave_sql_world.SqlSetRootOpId)
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_query_world.SqlQuerySetRootOp](t, s4wave_sql_query_world.SqlQuerySetRootOpId)
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_query_result_world.SqlQueryResultSetRootOp](t, s4wave_sql_query_result_world.SqlQueryResultSetRootOpId)
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_schema_world.SqlSchemaSetRootOp](t, s4wave_sql_schema_world.SqlSchemaSetRootOpId)
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_table_view_world.SqlTableViewSetRootOp](t, s4wave_sql_table_view_world.SqlTableViewSetRootOpId)
	requireLookupWorldAndBuildSpaceOp[*s4wave_sql_workbench_world.SqlWorkbenchSetRootOp](t, s4wave_sql_workbench_world.SqlWorkbenchSetRootOpId)
}
