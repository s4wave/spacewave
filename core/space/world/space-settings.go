package space_world

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
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

// MarshalBlock marshals the block to binary.
func (s *SpaceSettings) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (s *SpaceSettings) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

var _ block.Block = (*SpaceSettings)(nil)
