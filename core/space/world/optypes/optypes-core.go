//go:build !tinygo && !sql_lite

package optypes

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

func lookupCoreWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return world.LookupOpSlice([]world.LookupOp{
		s4wave_kv_world.LookupKvSetRootOp,
		s4wave_sql_world.LookupSqlSetRootOp,
		s4wave_sql_query_world.LookupSqlQuerySetRootOp,
		s4wave_sql_query_result_world.LookupSqlQueryResultSetRootOp,
		s4wave_sql_schema_world.LookupSqlSchemaSetRootOp,
		s4wave_sql_table_view_world.LookupSqlTableViewSetRootOp,
		s4wave_sql_workbench_world.LookupSqlWorkbenchSetRootOp,
		s4wave_sshhost.LookupCreateSshHostOp,
		s4wave_wizard.LookupCreateWizardObjectOp,
	}).LookupOp(ctx, opTypeID)
}
