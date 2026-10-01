package session

import (
	"cmp"
	"context"
	"slices"
)

// LookupSessionMetadata returns the metadata of the Session registered under
// ref, or nil if no registered Session matches.
func LookupSessionMetadata(
	ctx context.Context,
	ctrl SessionController,
	ref *SessionRef,
) (*SessionMetadata, error) {
	// Snapshot the registered Sessions from the metadata owner.
	entries, err := ctrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the matching entry through its controller-assigned index.
	for _, entry := range entries {
		if !entry.GetSessionRef().EqualVT(ref) {
			continue
		}
		return ctrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
	}
	return nil, nil
}

// SetBackgroundPlugin adds or removes the confirmation for one plugin of one
// Space, keeping BackgroundPlugins sorted and free of duplicates.
func (m *SessionMetadata) SetBackgroundPlugin(spaceID, pluginID string, enabled bool) {
	// Drop any existing confirmation for the pair.
	m.BackgroundPlugins = slices.DeleteFunc(m.BackgroundPlugins, func(bp *BackgroundPlugin) bool {
		return bp.GetSpaceId() == spaceID && bp.GetPluginId() == pluginID
	})
	if !enabled {
		return
	}

	// Insert the confirmation in Space and plugin order.
	m.BackgroundPlugins = append(m.BackgroundPlugins, &BackgroundPlugin{SpaceId: spaceID, PluginId: pluginID})
	slices.SortFunc(m.BackgroundPlugins, func(a, b *BackgroundPlugin) int {
		return cmp.Or(
			cmp.Compare(a.GetSpaceId(), b.GetSpaceId()),
			cmp.Compare(a.GetPluginId(), b.GetPluginId()),
		)
	})
}
