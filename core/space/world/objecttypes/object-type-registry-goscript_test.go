//go:build goscript

package objecttypes

import (
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
)

func TestLookupDeviceObjectTypeUnderGoScript(t *testing.T) {
	requireObjectType(t, s4wave_device.DeviceTypeID)
}

func TestLookupSqlObjectTypesUnderGoScript(t *testing.T) {
	for _, typeID := range []string{
		s4wave_sql_world.SqlDbTypeID,
		s4wave_sql_query.SqlQueryTypeID,
		s4wave_sql_query_result.SqlQueryResultTypeID,
		s4wave_sql_schema.SqlSchemaTypeID,
		s4wave_sql_table_view.SqlTableViewTypeID,
		s4wave_sql_workbench.SqlWorkbenchTypeID,
	} {
		requireObjectType(t, typeID)
	}
}
