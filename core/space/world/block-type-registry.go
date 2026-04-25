package space_world

import (
	"context"

	"github.com/s4wave/spacewave/hydra-exp/blocktype"
)

// LookupBlockType looks up a block type by ID.
// Returns nil if not found.
func LookupBlockType(ctx context.Context, typeID string) (blocktype.BlockType, error) {
	switch typeID {
	case "github.com/s4wave/spacewave/core/space/world.SpaceSettings":
		return SpaceSettingsBlockType, nil
	default:
		return nil, nil
	}
}
