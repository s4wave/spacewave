//go:build goscript

package objecttypes

import (
	"context"

	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_world "github.com/s4wave/spacewave/sdk/chat/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_device_world "github.com/s4wave/spacewave/sdk/device/world"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_query_result_world "github.com/s4wave/spacewave/sdk/sql/query-result/world"
	s4wave_sql_query_world "github.com/s4wave/spacewave/sdk/sql/query/world"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_schema_world "github.com/s4wave/spacewave/sdk/sql/schema/world"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_table_view_world "github.com/s4wave/spacewave/sdk/sql/table-view/world"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sql_workbench_world "github.com/s4wave/spacewave/sdk/sql/workbench/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_sshhost_world "github.com/s4wave/spacewave/sdk/sshhost/world"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_terminal_world "github.com/s4wave/spacewave/sdk/terminal/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// LookupObjectType looks up a GoScript-supported object type by ID.
// Returns nil if not found.
func LookupObjectType(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
	switch typeID {
	case s4wave_layout_world.ObjectLayoutTypeID:
		return s4wave_layout_world.ObjectLayoutType, nil
	case s4wave_unixfs_world.UnixFSTypeID:
		return s4wave_unixfs_world.UnixFSType, nil
	case s4wave_git_world.GitRepoTypeID:
		return s4wave_git_world.GitRepoType, nil
	case s4wave_canvas_world.CanvasTypeID:
		return s4wave_canvas_world.CanvasType, nil
	case s4wave_git_world.GitWorktreeTypeID:
		return s4wave_git_world.GitWorktreeType, nil
	case s4wave_kv_world.KvStoreTypeID:
		return s4wave_kv_world.KvStoreType, nil
	case s4wave_sql_world.SqlDbTypeID:
		return s4wave_sql_world.SqlDbType, nil
	case s4wave_sql_query.SqlQueryTypeID:
		return s4wave_sql_query_world.SqlQueryType, nil
	case s4wave_sql_query_result.SqlQueryResultTypeID:
		return s4wave_sql_query_result_world.SqlQueryResultType, nil
	case s4wave_sql_schema.SqlSchemaTypeID:
		return s4wave_sql_schema_world.SqlSchemaType, nil
	case s4wave_sql_table_view.SqlTableViewTypeID:
		return s4wave_sql_table_view_world.SqlTableViewType, nil
	case s4wave_sql_workbench.SqlWorkbenchTypeID:
		return s4wave_sql_workbench_world.SqlWorkbenchType, nil
	case spacewave_chat.ChatChannelTypeID:
		return spacewave_chat_world.ChatChannelType, nil
	case spacewave_chat.ChatMessageTypeID:
		return spacewave_chat_world.ChatMessageType, nil
	case s4wave_device.ComputersDashboardTypeID:
		return s4wave_device_world.ComputersDashboardType, nil
	case s4wave_terminal.TerminalTypeID:
		return s4wave_terminal_world.TerminalType, nil
	case s4wave_sshhost.SshHostTypeID:
		return s4wave_sshhost_world.SshHostType, nil
	default:
		return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
	}
}
