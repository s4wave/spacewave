package space_world_ops

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// InstallSpacePlugin installs a plugin in the Space, optionally pinning an
// immutable artifact. It reports whether the SpaceSettings changed.
//
// A pinned artifact moves to the front of the plugin's ordered installation
// list. Earlier artifacts stay behind it, so replacing the artifact of one
// platform keeps its predecessor and every other platform's artifact.
func InstallSpacePlugin(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	pluginID, manifestKey string,
	ts time.Time,
) (bool, error) {
	// Require a plugin identity.
	if pluginID == "" {
		return false, errors.New("plugin_id is required")
	}

	// Read the current settings, which may not exist yet.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		return false, err
	}
	if settings == nil {
		settings = &space_world.SpaceSettings{}
	}

	// A pinned artifact must be this plugin's immutable content.
	keys := settings.GetPluginInstallations()[pluginID].GetManifestKeys()
	if manifestKey != "" {
		if _, err := space_world.LookupSpacePluginManifest(ctx, ws, pluginID, manifestKey); err != nil {
			return false, err
		}
	}
	installed := slices.Contains(settings.PluginIds, pluginID)
	if installed && (manifestKey == "" || len(keys) != 0 && keys[0] == manifestKey) {
		return false, nil
	}

	// Record the plugin and move the pinned artifact to the front.
	if !installed {
		settings.PluginIds = append(settings.PluginIds, pluginID)
	}
	if manifestKey != "" {
		if settings.PluginInstallations == nil {
			settings.PluginInstallations = make(map[string]*space_world.SpacePluginInstallation)
		}
		keys = slices.DeleteFunc(keys, func(previous string) bool { return previous == manifestKey })
		settings.PluginInstallations[pluginID] = &space_world.SpacePluginInstallation{
			ManifestKeys: append([]string{manifestKey}, keys...),
		}
	}

	// Write the settings back through the operation.
	if _, _, err := SetSpaceSettings(ctx, ws, sender, DefaultSpaceSettingsObjectKey, settings, true, ts); err != nil {
		return false, err
	}
	return true, nil
}
