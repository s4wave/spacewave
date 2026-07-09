package space_world

import (
	"context"

	"github.com/s4wave/spacewave/db/blocktype"
)

// LookupBlockType looks up a core-owned block type by ID.
// Returns nil if not found. Plugin-owned block types (such as the SQL cursor
// blocks owned by the sql plugin) resolve through their plugin's
// LookupBlockType directive handler, not here.
func LookupBlockType(ctx context.Context, typeID string) (blocktype.BlockType, error) {
	switch typeID {
	case "github.com/s4wave/spacewave/core/space/world.SpaceSettings":
		return SpaceSettingsBlockType, nil
	default:
		return nil, nil
	}
}
