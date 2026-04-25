package resource_space

import (
	"context"
	"errors"
	"slices"
	"time"

	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// AddSpacePlugin adds a plugin manifest ID to the SpaceSettings plugin list.
func (r *SpaceResource) AddSpacePlugin(
	ctx context.Context,
	req *s4wave_space.AddSpacePluginRequest,
) (*s4wave_space.AddSpacePluginResponse, error) {
	pid := req.GetPluginId()
	if pid == "" {
		return nil, errors.New("plugin_id is required")
	}

	engine := r.space.GetWorldEngine()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read current settings (may be nil if not yet created).
	settings, _, err := space_world.LookupSpaceSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &space_world.SpaceSettings{}
	}

	// Add plugin ID if not already present.
	if slices.Contains(settings.PluginIds, pid) {
		return &s4wave_space.AddSpacePluginResponse{}, nil
	}
	settings.PluginIds = append(settings.PluginIds, pid)

	// Write back via SetSpaceSettings operation.
	_, _, err = space_world_ops.SetSpaceSettings(
		ctx, tx, "", space_world_ops.DefaultSpaceSettingsObjectKey,
		settings, true, time.Now(),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	r.le.Infof("added plugin %s to space settings", pid)
	return &s4wave_space.AddSpacePluginResponse{}, nil
}

// RemoveSpacePlugin removes a plugin manifest ID from the SpaceSettings plugin list.
func (r *SpaceResource) RemoveSpacePlugin(
	ctx context.Context,
	req *s4wave_space.RemoveSpacePluginRequest,
) (*s4wave_space.RemoveSpacePluginResponse, error) {
	pid := req.GetPluginId()
	if pid == "" {
		return nil, errors.New("plugin_id is required")
	}

	engine := r.space.GetWorldEngine()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	settings, _, err := space_world.LookupSpaceSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return &s4wave_space.RemoveSpacePluginResponse{}, nil
	}

	idx := slices.Index(settings.PluginIds, pid)
	if idx < 0 {
		return &s4wave_space.RemoveSpacePluginResponse{}, nil
	}
	settings.PluginIds = slices.Delete(settings.PluginIds, idx, idx+1)

	_, _, err = space_world_ops.SetSpaceSettings(
		ctx, tx, "", space_world_ops.DefaultSpaceSettingsObjectKey,
		settings, true, time.Now(),
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	r.le.Infof("removed plugin %s from space settings", pid)
	return &s4wave_space.RemoveSpacePluginResponse{}, nil
}
