package plugin_space

import (
	"context"
	"slices"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
)

// pluginLoads is the state the watch loop keeps for the plugins the Space
// loads. The watch loop goroutine owns it.
type pluginLoads struct {
	// refs is the load of each plugin, by plugin ID.
	refs map[string]pluginReference
	// hosted is the World selection of a hosted Space's unpinned plugins.
	hosted hostedManifests
	// pinned is each pinned plugin's resolved installation, by plugin ID.
	pinned map[string]pinnedManifests
	// gate admits the World manifests a load specifies.
	gate *admissionGate
}

// pinnedManifests is one plugin's resolved installation pin.
type pinnedManifests struct {
	// keys is the installation's manifest keys.
	keys []string
	// refs is the manifests the keys resolved to.
	refs []*manifest.ManifestRef
}

// lookupPinned returns the manifests a plugin's installation keys resolve to,
// skipping a key that does not resolve. An installation that resolved fully is
// kept until its keys change, so a pass rereads only a changed pin.
func (l *pluginLoads) lookupPinned(ctx context.Context, c *Controller, ws world.WorldState, pluginID string, keys []string) []*manifest.ManifestRef {
	// Reuse the installation resolved for the same keys.
	if prev, ok := l.pinned[pluginID]; ok && slices.Equal(prev.keys, keys) {
		return prev.refs
	}

	// Resolve each key, keeping the result only when every key resolved.
	refs := make([]*manifest.ManifestRef, 0, len(keys))
	for _, key := range keys {
		ref, err := space_world.LookupSpacePluginManifest(ctx, ws, pluginID, key)
		if err != nil {
			warnOnErrorUnlessCanceled(ctx, c.GetLogger(), err, "failed to resolve installed plugin artifact")
			continue
		}
		refs = append(refs, ref)
	}
	if len(refs) == len(keys) {
		l.pinned[pluginID] = pinnedManifests{keys: slices.Clone(keys), refs: refs}
	}
	return refs
}
