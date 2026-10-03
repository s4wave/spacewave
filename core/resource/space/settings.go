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
	// Install the plugin, and its artifact if pinned, in one transaction.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	changed, err := space_world_ops.InstallSpacePlugin(ctx, tx, "", req.GetPluginId(), req.GetManifestKey(), time.Now())
	if err != nil {
		return nil, err
	}
	if !changed {
		return &s4wave_space.AddSpacePluginResponse{}, nil
	}

	// Commit the new installation.
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.le.Infof("added plugin %s to space settings", req.GetPluginId())
	return &s4wave_space.AddSpacePluginResponse{}, nil
}

// RemoveSpacePlugin removes a plugin manifest ID from the SpaceSettings plugin list.
func (r *SpaceResource) RemoveSpacePlugin(
	ctx context.Context,
	req *s4wave_space.RemoveSpacePluginRequest,
) (*s4wave_space.RemoveSpacePluginResponse, error) {
	// Require a plugin identity before changing SpaceSettings.
	pid := req.GetPluginId()
	if pid == "" {
		return nil, errors.New("plugin_id is required")
	}

	// Open a write transaction for the plugin removal.
	engine := r.space.GetWorldEngine()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Read the current SpaceSettings before removing its plugin.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, tx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return &s4wave_space.RemoveSpacePluginResponse{}, nil
	}

	// Remove the plugin and its pinned installation from SpaceSettings.
	idx := slices.Index(settings.PluginIds, pid)
	if idx < 0 {
		return &s4wave_space.RemoveSpacePluginResponse{}, nil
	}
	settings.PluginIds = slices.Delete(settings.PluginIds, idx, idx+1)
	delete(settings.PluginInstallations, pid)

	// Write the revised SpaceSettings into the transaction.
	_, _, err = space_world_ops.SetSpaceSettings(
		ctx, tx, "", space_world_ops.DefaultSpaceSettingsObjectKey,
		settings, true, time.Now(),
	)
	if err != nil {
		return nil, err
	}

	// Publish the plugin removal by committing SpaceSettings.
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Report the completed Space plugin removal.
	r.le.Infof("removed plugin %s from space settings", pid)
	return &s4wave_space.RemoveSpacePluginResponse{}, nil
}
