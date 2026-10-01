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

// SetBackgroundPlugin replaces the confirmation for one plugin of one Space, or
// removes it when bp is nil, keeping BackgroundPlugins sorted and free of
// duplicates.
func (m *SessionMetadata) SetBackgroundPlugin(spaceID, pluginID string, bp *BackgroundPlugin) {
	// Drop any existing confirmation for the pair.
	m.BackgroundPlugins = slices.DeleteFunc(m.BackgroundPlugins, func(cur *BackgroundPlugin) bool {
		return cur.GetSpaceId() == spaceID && cur.GetPluginId() == pluginID
	})
	if bp == nil {
		return
	}

	// Insert the confirmation in Space and plugin order.
	m.BackgroundPlugins = append(m.BackgroundPlugins, bp)
	slices.SortFunc(m.BackgroundPlugins, func(a, b *BackgroundPlugin) int {
		return cmp.Or(
			cmp.Compare(a.GetSpaceId(), b.GetSpaceId()),
			cmp.Compare(a.GetPluginId(), b.GetPluginId()),
		)
	})
}
