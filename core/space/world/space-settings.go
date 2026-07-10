package space_world

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// SpaceSettingsObjectKey is the object key for the SpaceSettings.
const SpaceSettingsObjectKey = "settings"

// NewSpaceSettingsBlock constructs a new SpaceSettings block.
func NewSpaceSettingsBlock() block.Block {
	return &SpaceSettings{}
}

// LookupSpaceSettings looks up the SpaceSettings object in the world.
// Returns nil, nil, nil if the settings object does not exist.
func LookupSpaceSettings(ctx context.Context, ws world.WorldState) (*SpaceSettings, world.ObjectState, error) {
	settings, state, err := world.LookupObject[*SpaceSettings](
		ctx,
		ws,
		SpaceSettingsObjectKey,
		NewSpaceSettingsBlock,
	)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil, nil
	}
	return settings, state, err
}

// LookupSpaceIndexObjectType returns the durable ObjectType selected by
// SpaceSettings.index_path. Missing settings, an empty index, and stale index
// paths have no semantic type and return an empty string.
func LookupSpaceIndexObjectType(ctx context.Context, ws world.WorldState) (string, error) {
	settings, state, err := LookupSpaceSettings(ctx, ws)
	defer world.ReleaseObjectState(state)
	if err != nil {
		return "", err
	}
	if settings == nil {
		return "", nil
	}

	indexPath := strings.TrimPrefix(path.Clean("/"+settings.GetIndexPath()), "/")
	if after, ok := strings.CutPrefix(indexPath, "-/"); ok {
		indexPath = after
	}
	if objectKey, _, ok := strings.Cut(indexPath, "/-/"); ok {
		indexPath = objectKey
	} else {
		indexPath = strings.TrimSuffix(indexPath, "/-")
	}
	if indexPath == "" {
		return "", nil
	}
	metadata, err := world_types.GetObjectMetadataBatch(ctx, ws, []string{indexPath})
	if err != nil {
		return "", err
	}
	if len(metadata) == 0 {
		return "", nil
	}
	return metadata[0].TypeID, nil
}

// MarshalBlock marshals the block to binary.
func (s *SpaceSettings) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (s *SpaceSettings) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

var _ block.Block = (*SpaceSettings)(nil)
