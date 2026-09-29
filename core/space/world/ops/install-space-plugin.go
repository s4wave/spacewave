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
// list and retains one predecessor for its platform. Older pins for that
// platform are removed; every other platform's pins keep their relative order.
// Immutable manifests and build provenance remain in the World. The caller
// supplies the World transaction that commits the installation.
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

	// Retain the pinned artifact and its platform's immediate predecessor.
	keys := settings.GetPluginInstallations()[pluginID].GetManifestKeys()
	next := keys
	if manifestKey != "" {
		// Resolve the new pin's immutable artifact and platform.
		artifact, err := space_world.LookupSpacePluginManifest(ctx, ws, pluginID, manifestKey)
		if err != nil {
			return false, err
		}

		// Put the new pin first and filter earlier pins without reordering them.
		next = make([]string, 1, len(keys)+1)
		next[0] = manifestKey
		retainedPrevious := false
		for _, key := range keys {
			// Reinstalling the artifact moves its existing pin to the front.
			if key == manifestKey {
				continue
			}

			// Resolve each pin's platform through its immutable manifest.
			previous, err := space_world.LookupSpacePluginManifest(ctx, ws, pluginID, key)
			if err != nil {
				return false, err
			}

			// Keep one rollback pin for this platform and every foreign pin.
			if previous.GetMeta().GetPlatformId() == artifact.GetMeta().GetPlatformId() {
				if retainedPrevious {
					continue
				}
				retainedPrevious = true
			}
			next = append(next, key)
		}
	}

	// Leave an unchanged installation untouched.
	installed := slices.Contains(settings.PluginIds, pluginID)
	if installed && slices.Equal(keys, next) {
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
		settings.PluginInstallations[pluginID] = &space_world.SpacePluginInstallation{
			ManifestKeys: next,
		}
	}

	// Write the settings back through the operation.
	if _, _, err := SetSpaceSettings(ctx, ws, sender, DefaultSpaceSettingsObjectKey, settings, true, ts); err != nil {
		return false, err
	}
	return true, nil
}
