package space_world

import (
	"context"

	"github.com/s4wave/spacewave/db/blocktype"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
)

// LookupBlockType looks up a block type by ID.
// Returns nil if not found.
func LookupBlockType(ctx context.Context, typeID string) (blocktype.BlockType, error) {
	switch typeID {
	case "github.com/s4wave/spacewave/core/space/world.SpaceSettings":
		return SpaceSettingsBlockType, nil
	case s4wave_sql_query.SqlQueryBlockTypeID:
		return s4wave_sql_query.SqlQueryBlockType, nil
	case s4wave_sql_workbench.SqlWorkbenchBlockTypeID:
		return s4wave_sql_workbench.SqlWorkbenchBlockType, nil
	default:
		return nil, nil
	}
}
